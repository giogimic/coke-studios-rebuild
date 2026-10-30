package studio

import (
	"fmt"
	"io"
	"sort"
)

// --- Streaming mixer implementation ---

// trackSource is a decoded PCM source for a track's asset, with simple looping.
type trackSource struct {
	pcm       []byte
	pos       int
	loopBytes int
}

// readInto mixes this track into dst starting at offset. Returns bytes mixed.
func (ts *trackSource) readInto(dst []byte, offset int) int {
	if ts.loopBytes <= 0 || len(ts.pcm) == 0 || offset >= len(dst) {
		return 0
	}
	remain := len(dst) - offset
	mixed := 0
	for remain > 0 {
		copyLen := ts.loopBytes - ts.pos
		if copyLen > remain {
			copyLen = remain
		}
		if copyLen <= 0 {
			break
		}
		// Saturating add into dst (16-bit signed little endian stereo)
		for j := 0; j+3 < copyLen; j += 4 {
			al := int(int16(int(dst[offset+j]) | (int(dst[offset+j+1]) << 8)))
			ar := int(int16(int(dst[offset+j+2]) | (int(dst[offset+j+3]) << 8)))
			bl := int(int16(int(ts.pcm[ts.pos+j]) | (int(ts.pcm[ts.pos+j+1]) << 8)))
			br := int(int16(int(ts.pcm[ts.pos+j+2]) | (int(ts.pcm[ts.pos+j+3]) << 8)))
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
			dst[offset+j] = byte(uint16(sl) & 0xff)
			dst[offset+j+1] = byte((uint16(sl) >> 8) & 0xff)
			dst[offset+j+2] = byte(uint16(sr) & 0xff)
			dst[offset+j+3] = byte((uint16(sr) >> 8) & 0xff)
		}
		ts.pos += copyLen
		offset += copyLen
		remain -= copyLen
		mixed += copyLen
		if ts.pos >= ts.loopBytes {
			ts.pos = 0
		}
	}
	return mixed
}

type scheduledEvent struct {
	startBytes int
	loops      int
	// source asset name
	asset string
}

type streamMixer struct {
	sp         *StereoPlayer
	schedule   []scheduledEvent
	srcPCM     map[string]*trackSource
	totalBytes int
	pos        int
	closed     bool
}

func (m *streamMixer) Read(p []byte) (int, error) {
	if m.closed {
		return 0, io.EOF
	}
	if m.pos >= m.totalBytes {
		return 0, io.EOF
	}
	if remaining := m.totalBytes - m.pos; len(p) > remaining {
		p = p[:remaining]
	}
	// Zero fill the buffer
	for i := range p {
		p[i] = 0
	}
	// For the current range [pos, pos+len(p)), mix any active events
	start := m.pos
	end := m.pos + len(p)
	for i := range m.schedule {
		ev := &m.schedule[i]
		loopBytes := m.srcPCM[ev.asset].loopBytes
		if loopBytes <= 0 {
			continue
		}
		for l := 0; l < ev.loops; l++ {
			loopStart := ev.startBytes + l*loopBytes
			loopEnd := loopStart + loopBytes
			if loopEnd <= start || loopStart >= end {
				continue
			}
			// Overlap region
			ovStart := start
			if loopStart > ovStart {
				ovStart = loopStart
			}
			ovEnd := end
			if loopEnd < ovEnd {
				ovEnd = loopEnd
			}
			offset := ovStart - start
			// Set source position relative to loopStart
			m.srcPCM[ev.asset].pos = alignToFrameBytes(ovStart - loopStart)
			_ = m.srcPCM[ev.asset].readInto(p[:ovEnd-start], offset)
		}
	}
	m.pos += len(p)
	return len(p), nil
}

func (m *streamMixer) Close() error {
	m.closed = true
	return nil
}

// buildStreamMixer converts EDL tracks into a streaming mixer without big allocations.
func (sp *StereoPlayer) buildStreamMixer(tracks []cdTrack) (*streamMixer, error) {
	const beatIntervalMS = 500
	// Ensure schedule is sorted by start time for better cache locality
	for i := range tracks {
		sort.Slice(tracks[i].events, func(a, b int) bool { return tracks[i].events[a].startMS < tracks[i].events[b].startMS })
	}
	srcPCM := map[string]*trackSource{}
	schedule := make([]scheduledEvent, 0, len(tracks))
	needMS := 0
	for _, tr := range tracks {
		src, err := sp.getPCM(tr.asset)
		if err != nil {
			return nil, err
		}
		srcLen := alignToFrameBytes(len(src))
		if srcLen <= 0 {
			continue
		}
		srcPCM[tr.asset] = &trackSource{pcm: src, loopBytes: srcLen}
		for _, ev := range tr.events {
			startBeats := ev.startMS / beatIntervalMS
			beats := sp.lookupBeats(tr.asset)
			if beats > 0 && (startBeats%beats) != 0 {
				startBeats++
			}
			alignedMS := startBeats * beatIntervalMS
			startBytes := alignToFrameBytes(sp.msToBytes(alignedMS))
			schedule = append(schedule, scheduledEvent{startBytes: startBytes, loops: ev.loopCnt, asset: tr.asset})
			endMS := alignedMS + ev.loopCnt*((srcLen*1000)/sp.bytesPerSecond())
			if endMS > needMS {
				needMS = endMS
			}
		}
	}
	if needMS <= 0 {
		needMS = 1_000 // at least 1s
	}
	m := &streamMixer{
		sp:         sp,
		schedule:   schedule,
		srcPCM:     srcPCM,
		totalBytes: sp.msToBytes(needMS + 20),
	}
	return m, nil
}

// Seek sets the streaming position without replaying earlier audio.
func (m *streamMixer) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += int64(m.pos)
	case io.SeekEnd:
		offset += int64(m.totalBytes)
	default:
		return 0, fmt.Errorf("invalid seek origin")
	}
	if offset < 0 {
		return 0, fmt.Errorf("negative audio position")
	}
	if offset > int64(m.totalBytes) {
		offset = int64(m.totalBytes)
	}
	m.pos = alignToFrameBytes(int(offset))
	return int64(m.pos), nil
}
