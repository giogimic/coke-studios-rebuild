/* Recognising Shockwave and Flash files by their contents, wherever they
 * are and whatever they are called.
 *
 * Director files are RIFF-style containers. A file written on Windows is
 * little-endian and every four-letter tag in it is stored backwards, so the
 * same compressed cast begins XFIR....CDGF on a PC and RIFX....FGDC on a
 * Mac; the tags are normalised to the Mac spelling here. The length after
 * the magic is the file's size less eight, which is what lets a cast be cut
 * cleanly out of a browser's cache file or a raw disk.
 *
 * Flash files begin FWS (plain) or CWS (zlib after the first 8 bytes); the
 * length in the header is the uncompressed size, so a CWS movie's true end
 * is only known by decompressing it. */
#include "finder.h"
#include "inflate.h"

static int tag_is(const char *t, const char *want)
{
    return t[0] == want[0] && t[1] == want[1] && t[2] == want[2] && t[3] == want[3];
}

int sniff(const u8 *p, u32 n, Sniff *s, int strict)
{
    memset(s, 0, sizeof *s);
    if (n >= 16 && (!memcmp(p, "XFIR", 4) || !memcmp(p, "RIFX", 4))) {
        int le = p[0] == 'X';
        u32 len = le ? rd_le32(p + 4) : rd_be32(p + 4);
        char next[5];
        int i;
        for (i = 0; i < 4; i++) {
            s->codec[i] = (char)(le ? p[11 - i] : p[8 + i]);
            next[i] = (char)(le ? p[15 - i] : p[12 + i]);
        }
        s->codec[4] = next[4] = 0;
        s->kind = K_DIRECTOR;
        s->length = (u64)len + 8;
        if (tag_is(s->codec, "FGDC")) {
            s->what = "Shockwave cast";
            s->ext = "cct";
            s->afterburner = 1;
        } else if (tag_is(s->codec, "FGDM")) {
            s->what = "Shockwave movie";
            s->ext = "dcr";
            s->afterburner = 1;
        } else if (tag_is(s->codec, "MV93")) {
            s->what = "Director file";
            s->ext = "dir";
        } else if (tag_is(s->codec, "MC95")) {
            s->what = "Director cast";
            s->ext = "cst";
        } else if (tag_is(s->codec, "APPL")) {
            s->what = "Director projector data";
            s->ext = "dir";
            if (strict) return 0; /* only as a whole file: projectors are found by name */
        } else {
            s->kind = K_NONE;
            return 0;
        }
        if (len < 32 || s->length > MAX_FILE) return s->kind = K_NONE, 0;
        if (strict) {
            if (s->afterburner && !tag_is(next, "Fver")) return s->kind = K_NONE, 0;
            if (!s->afterburner && !tag_is(next, "imap")) return s->kind = K_NONE, 0;
        }
        return 1;
    }
    if (n >= 8 && (p[0] == 'F' || p[0] == 'C') && p[1] == 'W' && p[2] == 'S') {
        u32 len = rd_le32(p + 4);
        int v = p[3];
        if (v < 1 || v > (strict ? 12 : 50) || len < 21 || len > MAX_FILE) return 0;
        if (p[0] == 'C' && v < 6) return 0; /* compression arrived with Flash 6 */
        if (strict && p[0] == 'F' && (n < 9 || (p[8] >> 3) == 0)) return 0;
        s->kind = K_SWF;
        s->cws = p[0] == 'C';
        s->version = v;
        s->length = len;
        s->what = s->cws ? "Flash movie (compressed)" : "Flash movie";
        s->ext = "swf";
        return 1;
    }
    return 0;
}

/* Inflates every zlib stream inside a Director file. Each resource of a
 * compressed cast is its own stream, so this recovers the member names,
 * scripts and text members without parsing the container. */
static int inflate_all(const u8 *p, u32 n, Buf *out)
{
    u32 i, good = 0, cap = 48u << 20;
    for (i = 0; i + 8 < n && out->len < cap; i++) {
        u32 used = 0, before = out->len;
        int r;
        if (p[i] != 0x78 || !zlib_header_ok(p + i, n - i)) continue;
        r = inflate_zlib(p + i, n - i, out, cap, &used);
        if (r == 0 && out->len > before) {
            good++;
            buf_add(out, "\n", 1);
            i += used - 1;
        } else if (r == -2) {
            good++;
            break;
        } else {
            out->len = before;
        }
    }
    return (int)good;
}

int examine(const u8 *p, u32 n, const Sniff *s, int strict, Exam *x)
{
    memset(x, 0, sizeof *x);
    x->length = n;
    if (s->kind == K_DIRECTOR) {
        if (s->length < n) x->length = (u32)s->length;
        else if (s->length > n) x->damaged = 1;
        if (s->afterburner && inflate_all(p, x->length, &x->text) == 0) {
            x->damaged = 1;
            if (strict) return 0;
        }
        return 1;
    }
    if (s->kind == K_SWF && !s->cws) {
        if (s->length < n) x->length = (u32)s->length;
        else if (s->length > n) x->damaged = 1;
        /* every Flash movie ends with an End tag: two zero bytes */
        if (strict && (x->damaged || p[x->length - 1] || p[x->length - 2])) return 0;
        return 1;
    }
    if (s->kind == K_SWF && s->cws) {
        u32 used = 0;
        int r = inflate_zlib(p + 8, n - 8, &x->text, (u32)s->length - 8, &used);
        if (r == 0 && x->text.len == s->length - 8) {
            x->length = 8 + used;
            return 1;
        }
        if (strict) return 0;
        x->damaged = 1;
        return 1;
    }
    return 1;
}

void exam_free(Exam *x)
{
    buf_free(&x->text);
}
