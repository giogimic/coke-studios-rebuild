package studio

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	mrand "math/rand"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"

	webembed "gocoke/web"
)

// StereoPlayer provides minimal playback of the legacy 6-track sequencer format
// used in incoming CD_P packets (EDL string in s attribute). It loads the sound
// library mapping and schedules per-track WAV playback at 120BPM (500ms beat).
type StereoPlayer struct {
	ctx           *audio.Context
	assetToBeats  map[string]int    // asset -> beats per loop (e.g., 4, 8, 16)
	assetToPath   map[string]string // asset -> embedded FS path
	pcmByAsset    map[string][]byte // decoded PCM cached at ctx.SampleRate(); keys are lower-cased asset names
	pcmBytes      int               // total bytes across pcmByAsset
	pcmLimit      int               // eviction threshold in bytes
	mu            sync.Mutex
	cancelCurrent context.CancelFunc
	currentID     int
	masterVolume  float64 // 0..1
	activePlayers []*audio.Player
	baseVolumes   []float64 // per-player base volume (0..1)
}

func NewStereoPlayer() *StereoPlayer {
	// Use a common WebAudio-friendly sample rate.
	const sampleRate = 48000
	audioContext := audio.CurrentContext()
	if audioContext == nil { audioContext = audio.NewContext(sampleRate) }
	sp := &StereoPlayer{
		ctx:          audioContext,
		assetToBeats: make(map[string]int),
		assetToPath:  make(map[string]string),
		pcmByAsset:   make(map[string][]byte),
		pcmLimit:     32 << 20, // 32MB cache cap (adjust as needed)
		masterVolume: 1.0,
	}
	sp.loadSoundLibrary()
	return sp
}

// loadSoundLibrary parses 94_SoundLibraryData.txt and indexes WAV files.
func (sp *StereoPlayer) loadSoundLibrary() {
	// 1) Parse beats and asset ids from XML (categories and top-level sounds)
	beats := map[string]int{}
	dataPath := path.Clean("Lingo/soundlibrary/94_SoundLibraryData.txt")
	f, err := webembed.Files.Open(dataPath)
	if err != nil {
		log.Printf("Stereo: unable to open sound library data: %v", err)
	} else {
		defer f.Close()
		dec := xml.NewDecoder(f)
		for {
			tok, errTok := dec.Token()
			if errTok == io.EOF {
				break
			}
			if errTok != nil {
				log.Printf("Stereo: xml decode error: %v", errTok)
				break
			}
			switch se := tok.(type) {
			case xml.StartElement:
				if strings.EqualFold(se.Name.Local, "Sound") {
					var name string
					var asset string
					var beatsStr string
					for _, a := range se.Attr {
						sw := strings.ToLower(a.Name.Local)
						if sw == "name" {
							name = a.Value
						} else if sw == "asset" {
							asset = a.Value
						} else if sw == "beats" {
							beatsStr = a.Value
						}
					}
					if strings.TrimSpace(asset) != "" && strings.TrimSpace(beatsStr) != "" {
						if b, err := strconv.Atoi(beatsStr); err == nil {
							beats[asset] = b
						} else {
							log.Printf("Stereo: invalid beats for %s (%s): %v", name, asset, err)
						}
					}
				}
			}
		}
	}

	// 2) Index all WAV files under soundlibrary/ to map asset -> path
	files, err := webembed.Files.ReadDir("Lingo/soundlibrary")
	if err != nil {
		log.Printf("Stereo: unable to read soundlibrary dir: %v", err)
	} else {
		for _, de := range files {
			if de.IsDir() {
				continue
			}
			name := de.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".wav") {
				continue
			}
			asset := name[:len(name)-4] // strip .wav
			if idx := strings.Index(asset, "_"); idx >= 0 {
				asset = asset[idx+1:]
			}
			sp.assetToPath[asset] = path.Join("Lingo/soundlibrary", name)
			// Also provide a case-insensitive alias
			sp.assetToPath[strings.ToLower(asset)] = path.Join("Lingo/soundlibrary", name)
			// Provide trailing-underscore and non-underscore aliases
			if strings.HasSuffix(asset, "_") {
				alt := strings.TrimSuffix(asset, "_")
				if _, ok := sp.assetToPath[alt]; !ok {
					sp.assetToPath[alt] = path.Join("Lingo/soundlibrary", name)
					sp.assetToPath[strings.ToLower(alt)] = path.Join("Lingo/soundlibrary", name)
				}
			} else {
				alt := asset + "_"
				if _, ok := sp.assetToPath[alt]; !ok {
					sp.assetToPath[alt] = path.Join("Lingo/soundlibrary", name)
					sp.assetToPath[strings.ToLower(alt)] = path.Join("Lingo/soundlibrary", name)
				}
			}
		}
	}

	// 3) Store beats map (case-insensitively too)
	for k, v := range beats {
		sp.assetToBeats[k] = v
		sp.assetToBeats[strings.ToLower(k)] = v
	}
}

// cdTrackEvent represents one scheduled play of a sound starting at startMS.
type cdTrackEvent struct {
	asset   string
	volume  int // 0..100
	startMS int
	loopCnt int
}

type cdTrack struct {
	asset  string
	volume int
	events []cdTrackEvent
}

// PlayEDL parses the legacy EDL string (s attr) and plays it.
func (sp *StereoPlayer) PlayEDL(artist, songName, edl string) { sp.PlayEDLAt(artist, songName, edl, 0) }

func (sp *StereoPlayer) PlayEDLAt(artist, songName, edl string, offset time.Duration) {
	receivedAt := time.Now()
	sp.PrepareNewSong(true)
	sp.mu.Lock()
	// Cancel any existing playback
	if sp.cancelCurrent != nil {
		sp.cancelCurrent()
		sp.cancelCurrent = nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	sp.cancelCurrent = cancel
	sp.currentID++
	myID := sp.currentID
	sp.mu.Unlock()

	tracks := sp.parseEDL(edl)
	if len(tracks) == 0 {
		log.Printf("Stereo: no tracks parsed from EDL: %q", edl)
		return
	}

	log.Printf("Stereo: playing '%s' by %s (%d tracks)", songName, artist, len(tracks))

	// Sort events by start time within each track
	for i := range tracks {
		sort.Slice(tracks[i].events, func(a, b int) bool { return tracks[i].events[a].startMS < tracks[i].events[b].startMS })
	}

	// Build a streaming mixer that mixes all tracks on the fly
	mixer, err := sp.buildStreamMixer(tracks)
	if err != nil {
		log.Printf("Stereo: buildStreamMixer error: %v", err)
		return
	}
	var wg sync.WaitGroup
	startDelay := 20 * time.Millisecond
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
			_ = mixer.Close()
			return
		case <-time.After(startDelay):
		}
		if offset > 0 {
			_, _ = mixer.Seek(int64(sp.msToBytes(int((offset+time.Since(receivedAt)).Milliseconds()))), io.SeekStart)
		}
		p, err := audio.NewPlayer(sp.ctx, mixer)
		if err != nil {
			log.Printf("Stereo: NewPlayer error: %v", err)
			_ = mixer.Close()
			return
		}
		// Track master volume via activePlayers/baseVolumes (1.0 base), so SetMasterVolume keeps working
		p.SetVolume(sp.masterVolume)
		sp.mu.Lock()
		sp.activePlayers = append(sp.activePlayers, p)
		sp.baseVolumes = append(sp.baseVolumes, 1.0)
		sp.mu.Unlock()
		p.Play()
		for p.IsPlaying() {
			if ctx.Err() != nil {
				_ = p.Close()
				sp.removeActivePlayer(p)
				_ = mixer.Close()
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		_ = p.Close()
		sp.removeActivePlayer(p)
		_ = mixer.Close()
	}()

	// Cleanup when all events complete or cancelled
	go func() {
		wg.Wait()
		sp.mu.Lock()
		if sp.currentID == myID {
			sp.cancelCurrent = nil
		}
		sp.mu.Unlock()
	}()
}

// Stop immediately stops any current playback.
func (sp *StereoPlayer) Stop() {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if sp.cancelCurrent != nil {
		sp.cancelCurrent()
		sp.cancelCurrent = nil
	}
}

// PrepareNewSong stops current playback, closes active players and resets buffers.
// If clearCache is true, it also clears the decoded PCM cache.
func (sp *StereoPlayer) PrepareNewSong(clearCache bool) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	if sp.cancelCurrent != nil {
		sp.cancelCurrent()
		sp.cancelCurrent = nil
	}
	for _, p := range sp.activePlayers {
		_ = p.Close()
	}
	sp.activePlayers = nil
	sp.baseVolumes = nil
	sp.currentID++ // bump generation to invalidate any late goroutines
	if clearCache {
		sp.pcmByAsset = make(map[string][]byte)
		sp.pcmBytes = 0
	}
}

func (sp *StereoPlayer) lookupBeats(asset string) int {
	if b, ok := sp.assetToBeats[asset]; ok {
		return b
	}
	if b, ok := sp.assetToBeats[strings.ToLower(asset)]; ok {
		return b
	}
	return 0
}

// newPlayerForAsset returns a ready-to-play Player for the given asset.
func (sp *StereoPlayer) newPlayerForAsset(asset string) (*audio.Player, error) {
	// Return from cache if present
	if pcm, ok := sp.pcmByAsset[asset]; ok {
		p := audio.NewPlayerFromBytes(sp.ctx, pcm)
		return p, nil
	}
	if pcm, ok := sp.pcmByAsset[strings.ToLower(asset)]; ok {
		p := audio.NewPlayerFromBytes(sp.ctx, pcm)
		return p, nil
	}
	filePath, ok := sp.assetToPath[asset]
	if !ok {
		filePath, ok = sp.assetToPath[strings.ToLower(asset)]
		if !ok {
			return nil, fmt.Errorf("asset not found: %s", asset)
		}
	}
	f, err := webembed.Files.Open(filePath)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		return nil, err
	}
	stream, err := wav.DecodeWithSampleRate(sp.ctx.SampleRate(), bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	// Read entire stream into a buffer and cache it
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, stream); err != nil {
		return nil, err
	}
	pcm := buf.Bytes()
	// Track total bytes and evict if the cache grows too large (clear-all policy)
	added := len(pcm)
	if sp.pcmLimit > 0 && sp.pcmBytes+added > sp.pcmLimit {
		sp.pcmByAsset = make(map[string][]byte)
		sp.pcmBytes = 0
	}
	sp.pcmByAsset[asset] = pcm
	sp.pcmByAsset[strings.ToLower(asset)] = pcm
	sp.pcmBytes += added
	p := audio.NewPlayerFromBytes(sp.ctx, pcm)
	return p, nil
}

// parseEDL converts the legacy EDL string into tracks/events.
// Format example (simplified):
//
//	Get Daft,4,4,54,{^Daft,100,14,danceOne_bass00,{8,7998,{3,47988,{^Rocks,50,82,rockOne_guitar01,{6,31992,{...
func (sp *StereoPlayer) parseEDL(s string) []cdTrack {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, "^")
	if len(parts) == 0 {
		return nil
	}
	var tracks []cdTrack
	for idx := 1; idx < len(parts); idx++ {
		seg := parts[idx]
		if seg == "" {
			continue
		}
		tr := cdTrack{}
		metaEnd := strings.IndexRune(seg, '{')
		meta := seg
		rest := ""
		if metaEnd >= 0 {
			meta = seg[:metaEnd]
			rest = seg[metaEnd:]
		}
		metaFields := strings.Split(meta, ",")
		if len(metaFields) >= 4 {
			vol, _ := strconv.Atoi(strings.TrimSpace(metaFields[1]))
			asset := strings.TrimSpace(metaFields[3])
			tr.asset = asset
			tr.volume = vol
		}
		// Lingo scans by repeatedly finding '{', then taking the comma-separated
		// values immediately preceding the next '{'. We'll emulate that by
		// reading loopCount until the next comma, then startMS until the next comma.
		cursor := rest
		for {
			lb := strings.Index(cursor, "{")
			if lb < 0 {
				break
			}
			cursor = cursor[lb+1:]
			// parse loopCount
			cidx := strings.Index(cursor, ",")
			if cidx < 0 {
				break
			}
			lcStr := strings.TrimSpace(cursor[:cidx])
			cursor = cursor[cidx+1:]
			// parse startMS (ends at next comma or next '{' or end)
			endIdx := len(cursor)
			if k := strings.Index(cursor, ",{"); k >= 0 {
				endIdx = k
			} else if k := strings.Index(cursor, ","); k >= 0 {
				endIdx = k
			} else if k := strings.Index(cursor, "{"); k >= 0 {
				endIdx = k
			}
			stStr := strings.TrimSpace(cursor[:endIdx])
			// advance cursor
			cursor = cursor[endIdx:]
			// coerce values
			lc, _ := strconv.Atoi(lcStr)
			st, _ := strconv.Atoi(stStr)
			if lc > 0 {
				tr.events = append(tr.events, cdTrackEvent{asset: tr.asset, volume: tr.volume, startMS: st, loopCnt: lc})
			}
		}
		if tr.asset != "" && len(tr.events) > 0 {
			tracks = append(tracks, tr)
		}
	}
	return tracks
}

// Scene integration

// HandleCDPlay is invoked when a realtime CD_P message is received.
func (s *Scene) HandleCDPlay(avatar, artist, songName, edl string) { s.HandleCDPlayAt(avatar, artist, songName, edl, 0) }

func (s *Scene) HandleCDPlayAt(avatar, artist, songName, edl string, offset time.Duration) {
	if s == nil {
		return
	}
	// Lazy init stereo
	if s.stereo == nil {
		s.stereo = NewStereoPlayer()
	}
	log.Printf("Studio: CD_P by %s: '%s' from %s", avatar, songName, artist)
	s.stereo.PlayEDLAt(artist, songName, edl, offset)
}

// ShowNowPlayingInfo displays the voting infostand while a track is playing
func (s *Scene) ShowNowPlayingInfo(songName, ownerName string) {
	if s == nil || s.infoStand == nil {
		return
	}
	s.nowPlayingActive = true
	s.infoStand.nowPlayingActive = true
	s.infoStand.customTitle = "NOW PLAYING"
	body := songName
	if strings.TrimSpace(ownerName) != "" {
		body += "\nby: " + ownerName
	}
	s.infoStand.customBody = body
	// Reset vote state for new performance
	s.infoStand.nowPlayingVoted = false
	s.infoStand.nowPlayingVoteUp = false
	// Keep both owner (display name) and player (actual av) if different layers need it
	s.infoStand.nowPlayingOwner = ownerName
	// If we know the player AV (current avatar name), record via SetCurrentUserName() comparison; here we set later in realtime
	s.infoStand.visible = true
	s.infoStand.rebuildButtonsForAvatar()
}

// HideNowPlayingInfo hides the voting infostand when track ends
func (s *Scene) HideNowPlayingInfo() {
	if s == nil || s.infoStand == nil {
		return
	}
	// If we know who was performing, stop holding microphone on that avatar
	performer := strings.TrimSpace(s.infoStand.nowPlayingPlayer)
	if performer != "" {
		if s.avatarsByName != nil {
			if av, ok := s.avatarsByName[performer]; ok && av != nil && av.Engine != nil {
				av.Engine.IsCarrying = false
				av.Engine.IsDrinking = false
				// Remove the right item (microphone) from attributes so it stops rendering
				delete(av.Engine.Attributes, "ri")
			}
			// Reset mic cycle state
			s.lastMicDrinkTime = time.Time{}

		}
		// Clear stored performer name
		s.infoStand.nowPlayingPlayer = ""
	}

	s.nowPlayingActive = false
	s.infoStand.nowPlayingActive = false
	s.infoStand.customTitle = ""
	s.infoStand.customBody = ""
	s.infoStand.Clear()
}

// StopStereo stops any in-progress stereo playback.
func (s *Scene) StopStereo() {
	if s != nil && s.stereo != nil {
		s.stereo.Stop()
	}
}

func (sp *StereoPlayer) bytesPerSecond() int {
	return sp.ctx.SampleRate() * 2 * 2 // 16-bit stereo
}

func (sp *StereoPlayer) msToBytes(ms int) int {
	if ms <= 0 {
		return 0
	}
	bps := sp.bytesPerSecond()
	b := int(float64(bps) * float64(ms) / 1000.0)
	// Align to 4-byte stereo frame
	return b - (b % 4)
}

// SetMasterVolume updates the global master volume (0..1) and reapplies it to active players.
func (sp *StereoPlayer) SetMasterVolume(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	sp.mu.Lock()
	sp.masterVolume = v
	// Reapply to all active players using stored base volumes
	for i, p := range sp.activePlayers {
		if i < len(sp.baseVolumes) && p != nil {
			p.SetVolume(sp.masterVolume * sp.baseVolumes[i])
		}
	}
	sp.mu.Unlock()
}

// removeActivePlayer removes a player from activePlayers safely.
func (sp *StereoPlayer) removeActivePlayer(p *audio.Player) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	for i, ap := range sp.activePlayers {
		if ap == p {
			// remove and keep order (small slice)
			copy(sp.activePlayers[i:], sp.activePlayers[i+1:])
			sp.activePlayers = sp.activePlayers[:len(sp.activePlayers)-1]
			break
		}
	}
}

// SetMuted is a helper to toggle mute on/off.
func (sp *StereoPlayer) SetMuted(muted bool) {
	if muted {
		sp.SetMasterVolume(0)
	} else {
		sp.SetMasterVolume(1)
	}
}

func alignToFrameBytes(v int) int { // 4 bytes per stereo frame
	if v < 0 {
		return 0
	}
	return v - (v % 4)
}

func (sp *StereoPlayer) getPCM(asset string) ([]byte, error) {
	if pcm, ok := sp.pcmByAsset[asset]; ok {
		return pcm, nil
	}
	if pcm, ok := sp.pcmByAsset[strings.ToLower(asset)]; ok {
		return pcm, nil
	}
	// Load into cache via newPlayerForAsset path
	p, err := sp.newPlayerForAsset(asset)
	if err != nil {
		return nil, err
	}
	_ = p.Close()
	if pcm, ok := sp.pcmByAsset[asset]; ok {
		return pcm, nil
	}
	if pcm, ok := sp.pcmByAsset[strings.ToLower(asset)]; ok {
		return pcm, nil
	}
	return nil, fmt.Errorf("pcm not cached for %s", asset)
}

// PlayOneShotAsset plays a single WAV asset immediately at the current master volume.
func (sp *StereoPlayer) PlayOneShotAsset(asset string) {
	if sp == nil {
		return
	}
	// Reuse cached PCM if available via newPlayerForAsset
	p, err := sp.newPlayerForAsset(asset)
	if err != nil {
		log.Printf("Stereo: PlayOneShotAsset failed for %s: %v", asset, err)
		return
	}
	p.SetVolume(sp.masterVolume)
	sp.mu.Lock()
	sp.activePlayers = append(sp.activePlayers, p)
	sp.baseVolumes = append(sp.baseVolumes, 1.0)
	sp.mu.Unlock()
	go func(pl *audio.Player) {
		pl.Play()
		for pl.IsPlaying() {
			time.Sleep(10 * time.Millisecond)
		}
		_ = pl.Close()
	}(p)
}

// HandleCDPerformanceResult selects and plays a reaction sound based on vote delta.
func (s *Scene) HandleCDPerformanceResult(thumbsUp, thumbsDown int) {
	if s == nil {
		return
	}
	// Respect mute
	if s.hudManager != nil && s.hudManager.IsSoundMuted() {
		return
	}
	diff := thumbsUp - thumbsDown
	var asset string
	if diff <= -16 {
		asset = "minus_16"
	} else if diff <= -10 {
		asset = "minus_10"
	} else if diff < 0 {
		// pick random of minus_1t1/2/3
		choices := []string{"minus_1t1", "minus_1t2", "minus_1t3"}
		if s.rng == nil {
			s.rng = mrand.New(mrand.NewSource(time.Now().UnixNano()))
		}
		asset = choices[s.rng.Intn(len(choices))]
	} else if diff >= 16 {
		asset = "plus_16"
	} else if diff >= 10 {
		asset = "plus_10"
	} else if diff >= 5 {
		asset = "plus_5"
	} else if diff >= 3 {
		asset = "plus_3"
	} else if diff >= 1 {
		asset = "plus_1"
	} else {
		// diff == 0: no reaction sound
		return
	}
	// Lazy init stereo player
	if s.stereo == nil {
		s.stereo = NewStereoPlayer()
	}
	s.stereo.PlayOneShotAsset(asset)
}

func (sp *StereoPlayer) renderTrack(tr cdTrack) ([]byte, error) {
	// Sort events
	sort.Slice(tr.events, func(a, b int) bool { return tr.events[a].startMS < tr.events[b].startMS })
	beatIntervalMS := 500
	// Determine minimal required buffer length based on events and clip duration
		src, err := sp.getPCM(tr.asset)
		if err != nil {
			return nil, err
		}
		srcLen := alignToFrameBytes(len(src))
		if srcLen <= 0 {
			return nil, fmt.Errorf("empty pcm for %s", tr.asset)
		}
		bps := sp.bytesPerSecond()
		clipMS := (srcLen * 1000) / bps
		needMS := 0
		for _, ev := range tr.events {
			startBeats := ev.startMS / beatIntervalMS
			beats := sp.lookupBeats(tr.asset)
			if beats > 0 && (startBeats%beats) != 0 {
				startBeats++
			}
			alignedMS := startBeats * beatIntervalMS
			endMS := alignedMS + ev.loopCnt*clipMS
			if endMS > needMS {
				needMS = endMS
			}
		}
		if needMS <= 0 {
			needMS = clipMS
		}
		needMS += 20 // small tail
		totalBytes := sp.msToBytes(needMS)
		dst := make([]byte, totalBytes)



	for _, ev := range tr.events {
		// Align start to beat boundary per Lingo
		startBeats := ev.startMS / beatIntervalMS
		beats := sp.lookupBeats(tr.asset)
		if beats > 0 && (startBeats%beats) != 0 {
			startBeats++
		}
		alignedMS := startBeats * beatIntervalMS
		startBytes := alignToFrameBytes(sp.msToBytes(alignedMS))

		// Use actual clip duration as loop step, like Lingo member.duration

		srcLen := alignToFrameBytes(len(src))
		loopStepBytes := srcLen
		if srcLen <= 0 {
			continue
		}

		for i := 0; i < ev.loopCnt; i++ {
			pos := startBytes + i*loopStepBytes
			if pos >= totalBytes {
				break
			}
			copyLen := srcLen
			if pos+copyLen > totalBytes {
				copyLen = totalBytes - pos
				copyLen = alignToFrameBytes(copyLen)
			}
			// Mix src into dst with saturation (16-bit signed little endian stereo)
			for j := 0; j+3 < copyLen; j += 4 {
				// existing left
				al := int(int16(int(dst[pos+j]) | (int(dst[pos+j+1]) << 8)))
				// existing right
				ar := int(int16(int(dst[pos+j+2]) | (int(dst[pos+j+3]) << 8)))
				// new left
				bl := int(int16(int(src[j]) | (int(src[j+1]) << 8)))
				// new right
				br := int(int16(int(src[j+2]) | (int(src[j+3]) << 8)))
				// sum with saturation
				sl := al + bl
				if sl > 32767 {
					sl = 32767
				} else if sl < -32768 {
					sl = -32768
				}
				sr := ar + br
				if sr > 32767 {
					sr = 32767
				} else if sr < -32768 {
					sr = -32768
				}
				dst[pos+j] = byte(uint16(sl) & 0xff)
				dst[pos+j+1] = byte((uint16(sl) >> 8) & 0xff)
				dst[pos+j+2] = byte(uint16(sr) & 0xff)
				dst[pos+j+3] = byte((uint16(sr) >> 8) & 0xff)
			}
		}
	}
	return dst, nil
}
