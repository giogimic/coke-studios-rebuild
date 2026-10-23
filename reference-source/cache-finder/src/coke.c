/* What makes a file Coke Studios', and which finds the project is hoping
 * for. Everything specific to the game is in this file. */
#include "finder.h"
#include "known.h"

/* Words that only Coke Studios / MyCoke files contain: the sites, the
 * company that built V2 (Studiocom, whose project folder "SFClient" is
 * stamped into most casts), and the client's own script and member names -
 * misspelling included. Chosen by checking every file in the archive: all
 * but two of its 134 Director files contain one, and those two contain the
 * engine's member names (hiliter_studio, cc.bubble). Habbo's engine shares
 * memberalias.index, h_std_ and pathmaps, so those are not here. */
static const char *const STRONG[] = {
    "cokemusic", "cokestudio", "coke studio", "coke music", "mycoke", "studiocom",
    "denizenmanager", "possessionmanager", "studiomanager", "sf_gateway", "v-ego",
    "vego", "freshtracks", "sfclient", "drink_despenser", "isoengine", "gfeatureset",
    "soundlibrarydata", "hiliter_studio", "studio.window.", "cc.bubble", "cc.infostand",
    "cc.backpack", 0};

/* Words that make a Shockwave or Flash file likely enough to be worth
 * keeping: the brand, and the MyCoke Flash team's habits (_bDebug, their
 * oController) and servers (ElectroServer, OddCast). Most of the website's
 * movies contain one of these and nothing stronger; the mixer's samples
 * contain no text at all and are only recognised by their address. */
static const char *const WEAK[] = {"coke", "coca-cola", "cocacola", "coca cola", "_bdebug",
                                   "ocontroller", "electroserver", "oddcast", 0};

/* Config files the client downloaded beside its casts. */
static const char *const TEXT_MARKERS[] = {"<pathmap", "soundlibrarydata", "colorsets", "[ navigator", 0};

/* The game's own sites. Anything a browser cached from these is kept,
 * whatever it is - casts, Flash, pictures, pages. audiomixer.oddcast.com
 * served the music mixer; blogs.icoke.com hosted the MyCoke Blogs.
 * (FreshTracks / myTracks, the Infinite Jukebox's music source, is left out:
 * it was an outside streaming service, not Coke Studios.) */
static const char *const HOSTS[] = {
    "cokemusic.com", "cokestudios.com", "cokestudio.com", "mycoke.com", "studiocom.com",
    "audiomixer.oddcast.com", "blogs.icoke.com", 0};

/* The same, as they appear inside a larger file (searched for while
 * carving, lowercase). */
static const char *const HOST_WORDS[] = {
    "cokemusic.", "cokestudios.", "cokestudio.", "mycoke.", "studiocom.",
    "audiomixer.oddcast.", "blogs.icoke.", 0};

/* Lost media, matched against a new find's name and address: what the
 * project knows existed but has no copy of. The later MyCoke website and
 * mobile games (Champrun, Celebra El Gol, Desktop Kicks and Hoops) are not
 * here: they ran on their own sites and were not part of Coke Studios. */
static const struct {
    const char *word;
    const char *label;
} WANTED[] = {
    {"movietheatre", "the Coming Soon Theatre with the Batman Begins poster dispenser (public room 24)"},
    {"walmart", "Wal-Mart Music Downloads (public room 25)"},
    {"auditiongreen", "a green Casting Call room, known only from the client's code (public room 21)"},
    {"auditionorange", "an orange Casting Call room, known only from the client's code (public room 28)"},
    {"atlantis2", "Neptune II's original cast (public room 29); the only copy known is a fan-made one"},
    {"/sanfrancisco/", "the Coke Studios 2.0 beta (2003-2004), which was never archived"},
    /* the Promo Poster's pictures: content/leftwall poster <name>.swf and its
     * rightwall twin. "make it real" survives (and is in known.c); the three
     * Shark Tale designs of 2004 do not. Before the Shark Tale words, so a
     * Shark Tale poster is named as one. */
    {"wall poster ", "a Promo Poster design (its three Shark Tale ones are missing)"},
    {"wall%20poster%20", "a Promo Poster design (its three Shark Tale ones are missing)"},
    {"washbuilder", "Whale Wash Builder, the first Shark Tale game"},
    {"/shark/", "the Shark Tale promotion (cokemusic.com/shark/)"},
    {"whalewash", "Whale Wash, the Shark Tale game Recycler was made from"},
    {"whale_wash", "Whale Wash, the Shark Tale game Recycler was made from"},
    {"sharktale", "the Shark Tale promotion"},
    {"shark_tale", "the Shark Tale promotion"},
    {"coasteranim", "Coaster's winning cutscene (CoasterAnim.swf)"},
    {"/cards/", "an Uncover the Music tile face"},
    {"/audio/stream", "a Sports Challenge announcer line"},
    {"/popquiz/", "Pop Quiz, the original 2002 trivia game by GamesThatWork"},
    {"dynamicpostercontent", "the Promo Poster's content"},
    /* The five row palettes survive, baked into the website's avatar_engine.swf
     * (AvatarConstants._acolors); the subsets some styles used of their own
     * (2, 3, 6, 7, 9, 10, 12, 13, 14) were only ever in this server file. */
    {"avatarconfig", "the V-ego creator's colour file (AvatarConfig): some styles' own palettes are still missing"},
    {"publicstudios", "an older public room list (it may hold room descriptions that are lost)"},
    {"cc_interface", "another build of the interface cast (the archive's lacks Ray's Rooftop's navigator icon)"},
    {"savemix", "a saved music mix from the OddCast mixer"},
    {"getdefaultmixes", "the OddCast mixer's preset mixes"},
    {"screensaver", "a Coke Music / MyCoke screensaver"},
    {"screen saver", "a Coke Music / MyCoke screensaver"},
    {".scr_at_", "a Coke Music / MyCoke screensaver"},
    {0, 0}};

static int any_in(const u8 *p, u32 n, const char *const *words, Str *found)
{
    int hits = 0, i;
    for (i = 0; words[i]; i++) {
        if (!mem_ifind(p, n, words[i])) continue;
        if (found && found->s) { /* already listed from another part of the file */
            Str q = {0};
            int dup;
            str_fmt(&q, "\"%s\"", words[i]);
            dup = str_ifind(found->s, q.s) != NULL;
            str_free(&q);
            if (dup) {
                hits++;
                continue;
            }
        }
        if (found) {
            if (found->len) str_add(found, ", ");
            str_addc(found, '"');
            str_add(found, words[i]);
            str_addc(found, '"');
        }
        hits++;
    }
    return hits;
}

void find_markers(const u8 *p, u32 n, Markers *m)
{
    m->strong += any_in(p, n, STRONG, &m->words);
    if (!m->strong) m->weak += any_in(p, n, WEAK, &m->words);
}

void markers_free(Markers *m)
{
    str_free(&m->words);
}

int known_text_marker(const u8 *p, u32 n)
{
    return any_in(p, n, STRONG, NULL) + any_in(p, n, TEXT_MARKERS, NULL);
}

/* ---- naming a file from its own contents ---- */

/* The public rooms, by the asset name the engine gives each one. That name
 * prefixes every member in the room's cast - a London cast is full of
 * `london_chair_...`, `london.room`, `cc.london.bg` - so a cast carved out
 * of disk space with no file name can still be told apart by counting which
 * room's prefix it carries. The lost rooms carry a WANTED line; for those we
 * have no cast to confirm the prefix, but the engine builds member names the
 * same way for every room (`getImageBase` + "_"), so `walmart_...` is what a
 * Wal-Mart cast would hold. More specific names (atlantis2, redroom2) come
 * before the shorter ones they contain. */
static const struct {
    const char *token;
    const char *label;
    const char *wanted;
} ROOMS[] = {
    /* Neptune II's only copy, decibel.fun's, is fan-made: Neptune I's cast
     * member for member with the Neptune II background swapped in. So it is
     * still wanted, and that copy is in known.c so it is not reported as the
     * original. Red Room II's copy from there is genuine (it carries the
     * original build path), so a carved one is named but not flagged wanted. */
    {"atlantis2", "Neptune II", "Neptune II's original cast (public room 29); the only copy known is a fan-made one"},
    {"redroom2", "Coca-Cola Red Room II", 0},
    {"movietheatre", "the Coming Soon Theatre", "the Coming Soon Theatre with the Batman Begins poster dispenser (public room 24)"},
    {"walmart", "Wal-Mart Music Downloads", "Wal-Mart Music Downloads (public room 25)"},
    {"auditiongreen", "Casting Call (green)", "a green Casting Call room, known only from the client's code (public room 21)"},
    {"auditionorange", "Casting Call (orange)", "an orange Casting Call room, known only from the client's code (public room 28)"},
    {"raysrooftopparty", "Ray's Rooftop Party", 0},
    {"castingcallroom", "the Casting Call room", 0},
    {"auditiongold", "the Casting Call room (gold)", 0},
    {"auditionred", "the Casting Call room (red)", 0},
    {"secretroom", "Whale Wash", 0},
    {"sanfrancisco", "the San Francisco room", 0},
    {"centralpark", "the Central Park room", 0},
    {"neworleans", "the New Orleans room", 0},
    {"clubcherry", "Club Cherry", 0},
    {"london", "the London room", 0},
    {"miami", "the Miami room", 0},
    {"mombasa", "the Mombasa room", 0},
    {"newyork", "the New York room", 0},
    {"seattle", "the Seattle room", 0},
    {"sydney", "the Sydney room", 0},
    {"tokyo", "the Tokyo room", 0},
    {"moscow", "the Moscow room", 0},
    {"alaska", "the Alaska room", 0},
    {"mexico", "the Mexico room", 0},
    {"atlantis", "the Neptune room", 0},
    {"redroom", "the Coca-Cola Red Room", 0},
    {"ncaa", "the NCAA Arena", 0},
    {"goa", "the Goa room", 0},
    {"rio", "the Rio room", 0},
    {0, 0, 0}};

/* Big files that are not rooms but carry an unmistakable string of their
 * own. The first marker that is present names the file. */
static const struct {
    const char *marker;
    const char *also;  /* a second marker that must also be present, or NULL */
    const char *label;
} CONTENTS[] = {
    {"soundlibrarydata", 0, "the music sample library (soundlibrary)"},
    {"#prodid:", "#catname:", "the shop's catalogue (Catalogue_English)"},
    {"studioid=\"$system$", 0, "the public room list (PublicStudios)"},
    {"whale_wash=", 0, "the game's text strings (Text_English)"},
    {"alert_mod_message", 0, "the game's text strings (Text_English)"},
    {"[ navigator", 0, "the game's text strings (Text_English)"},
    {"h_std_hr_", 0, "the V-ego character art (people)"},
    {"receiveavatarenter", 0, "the SF_Gateway network cast"},
    {"loadoddcastsequence", 0, "the SF_Gateway network cast"},
    {"cc_entry.window", 0, "the main client movie (SF_Client)"},
    {"loading sharktale", 0, "the Shark Tale client movie (SF_Client)"},
    {"fuseclient(tm)", 0, "a version 1 client movie (fuse_client)"},
    {"editavatarmanager", 0, "the Moderator Tool (modTool)"},
    {"wallpattern_patterns_index", 0, "the private-studio shell art (cc_studio)"},
    {0, 0, 0}};

static u32 count_token(const u8 *p, u32 n, const char *word)
{
    u32 k = (u32)str_len(word), i, hits = 0;
    if (k == 0 || n < k) return 0;
    for (i = 0; i + k <= n; i++)
        if (lower(p[i]) == (u8)word[0] && str_istarts((const char *)p + i, word)) {
            hits++;
            i += k - 1;
        }
    return hits;
}

/* Builds "<token>_", the member-name prefix, into a small buffer. */
static int member_prefix(char *out, int cap, const char *token)
{
    int n = (int)str_len(token);
    if (n + 2 > cap) return 0;
    memcpy(out, token, (size_t)n);
    out[n] = '_';
    out[n + 1] = 0;
    return 1;
}

const char *identify_content(const u8 *p, u32 n, const u8 *text, u32 tn, const char **out_wanted)
{
    u32 best = 0;
    int bestRoom = -1, i;
    char prefix[64];
    if (out_wanted) *out_wanted = NULL;

    /* a room: whichever asset prefix appears most, over both the raw bytes
     * (for a plain cast) and the decompressed text (for a compressed one) */
    for (i = 0; ROOMS[i].token; i++) {
        u32 c;
        if (!member_prefix(prefix, sizeof prefix, ROOMS[i].token)) continue;
        c = count_token(p, n, prefix);
        if (text && tn) c += count_token(text, tn, prefix);
        if (c > best) {
            best = c;
            bestRoom = i;
        }
    }
    if (bestRoom >= 0 && best >= 3) {
        if (out_wanted) *out_wanted = ROOMS[bestRoom].wanted;
        return ROOMS[bestRoom].label;
    }

    /* otherwise one of the big named files */
    for (i = 0; CONTENTS[i].marker; i++) {
        int in_raw = mem_ifind(p, n, CONTENTS[i].marker) != NULL;
        int in_text = text && tn && mem_ifind(text, tn, CONTENTS[i].marker);
        if (!in_raw && !in_text) continue;
        if (CONTENTS[i].also) {
            if (!mem_ifind(p, n, CONTENTS[i].also) && !(text && tn && mem_ifind(text, tn, CONTENTS[i].also)))
                continue;
        }
        return CONTENTS[i].label;
    }
    return NULL;
}

/* Turns a label into a file-name stem: lower case, spaces to hyphens, a
 * leading "the " dropped, only letters, digits and hyphens kept. */
char *label_slug(const char *label)
{
    Str b = {0};
    const char *p = label;
    int last_dash = 1;
    if (str_istarts(p, "the ")) p += 4;
    for (; *p && b.len < 40; p++) {
        int c = lower((u8)*p);
        if ((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
            str_addc(&b, (char)c);
            last_dash = 0;
        } else if (!last_dash) {
            str_addc(&b, '-');
            last_dash = 1;
        }
    }
    while (b.len && b.s[b.len - 1] == '-') b.s[--b.len] = 0;
    return b.s ? b.s : str_dup("file");
}

void url_host(const char *url, char *out, int cap)
{
    const char *p = str_ifind(url, "://");
    int n = 0;
    p = p ? p + 3 : url;
    for (; *p && *p != '/' && *p != ':' && *p != '?' && *p != '#' && *p != '\\'; p++) {
        if (*p == '@') { /* user@host */
            n = 0;
            continue;
        }
        if (n < cap - 1) out[n++] = (char)lower((u8)*p);
    }
    out[n] = 0;
}

int coke_url(const char *url)
{
    char host[256];
    int i;
    if (!url) return 0;
    url_host(url, host, sizeof host);
    for (i = 0; HOSTS[i]; i++) {
        size_t a = str_len(host), b = str_len(HOSTS[i]);
        if (a == b && str_eq(host, HOSTS[i])) return 1;
        if (a > b && host[a - b - 1] == '.' && str_eq(host + a - b, HOSTS[i])) return 1;
    }
    return 0;
}

int match_host_at(const u8 *p, u32 avail)
{
    int i;
    for (i = 0; HOST_WORDS[i]; i++) {
        const char *w = HOST_WORDS[i];
        u32 k = 0;
        while (w[k] && k < avail && lower(p[k]) == (u8)w[k]) k++;
        if (!w[k]) return (int)k;
    }
    return 0;
}

const char *wanted_label(const char *name, const char *url)
{
    Str hay = {0};
    const char *found = NULL;
    int i;
    str_add(&hay, name ? name : "");
    str_addc(&hay, ' ');
    str_add(&hay, url ? url : "");
    for (i = 0; WANTED[i].word && !found; i++)
        if (str_ifind(hay.s, WANTED[i].word)) found = WANTED[i].label;
    /* STREAM<type>_NN.SWF: the quiz host's voice lines */
    if (!found && name && str_istarts(name, "stream") && str_iends(name, ".swf"))
        found = "a Sports Challenge announcer line";
    str_free(&hay);
    return found;
}

/* A few lost files hold no text at all, so nothing inside them says Coke
 * Studios: the Promo Poster's pictures are bare art. Their names are the
 * evidence instead, and only names nothing else would have. */
int name_identifies(const char *name)
{
    static const char *const NAMES[] = {"leftwall poster ", "rightwall poster ", "leftwall%20poster%20",
                                        "rightwall%20poster%20", 0};
    int i;
    if (!name) return 0;
    for (i = 0; NAMES[i]; i++)
        if (str_ifind(name, NAMES[i])) return 1;
    return 0;
}

int name_is_cokeish(const char *name)
{
    if (str_istarts(name, "cokecachefinder") || str_istarts(name, "cokestudiosfinds")) return 0;
    return str_ifind(name, "coke") || str_ifind(name, "v-ego") || str_ifind(name, "vego") ||
           str_ifind(name, "studiocom");
}

int text_name_wanted(const char *name)
{
    static const char *const NAMES[] = {"text_", "catalogue_", "publicstudios", "avatarconfig",
                                        "memberalias", "dynamicpostercontent", "soundlibrary", 0};
    const char *ext = path_ext(name);
    int i;
    /* the client fetched these as .txt and .xml (IE keeps the extension;
     * other caches drop it) */
    if (*ext && !str_ieq(ext, "txt") && !str_ieq(ext, "xml") && !str_iends(ext, "]")) return 0;
    for (i = 0; NAMES[i]; i++)
        if (str_ifind(name, NAMES[i])) return 1;
    return 0;
}

/* Folders whose files are a browser's or plugin's leftovers, and so worth
 * searching inside rather than only by their first bytes. */
int cachey_dir(const char *name)
{
    static const char *const PARTS[] = {
        "cache", "temporary internet files", "content.ie5", "inetcache", "netscape",
        "mozilla", "opera", "safari", "macromed", "shockwave", "dswmedia", "webcache",
        "history", "offline web pages", 0};
    static const char *const EXACT[] = {"temp", "tmp", "low", 0};
    int i;
    for (i = 0; PARTS[i]; i++)
        if (str_ifind(name, PARTS[i])) return 1;
    for (i = 0; EXACT[i]; i++)
        if (str_ieq(name, EXACT[i])) return 1;
    return 0;
}

/* ---- the archive's own files, by SHA-1 ---- */

static int hexcmp(const char *a, const char *b)
{
    int i;
    for (i = 0; i < 40; i++)
        if (a[i] != b[i]) return (u8)a[i] - (u8)b[i];
    return 0;
}

const char *known_name(const char *sha1hex, u64 size)
{
    int lo = 0, hi = KNOWN_FILES_COUNT - 1;
    while (lo <= hi) {
        int mid = (lo + hi) / 2, c = hexcmp(sha1hex, KNOWN_FILES[mid].sha1);
        if (c == 0) return KNOWN_FILES[mid].size == size ? KNOWN_FILES[mid].name : NULL;
        if (c < 0) hi = mid - 1;
        else lo = mid + 1;
    }
    return NULL;
}

int known_count(void)
{
    return KNOWN_FILES_COUNT;
}
