/* DEFLATE decoding (RFC 1951) with the zlib wrapper (RFC 1950).
 *
 * Shockwave casts compress every resource with zlib, and compressed Flash
 * movies (CWS) are one zlib stream after an 8-byte header, so this is how
 * the finder looks inside them for the words that say "Coke Studios", and
 * how it learns where a compressed movie found inside a bigger file ends.
 * A plain canonical-Huffman decoder: small and easy to check, which matters
 * more here than speed. */
#include "inflate.h"

typedef struct {
    const u8 *in;
    u32 inlen, pos;
    u32 bitbuf, bitcnt;
    Buf *out;
    u32 maxout;
} State;

typedef struct {
    u16 count[16];
    u16 symbol[320];
} Huff;

enum { E_INPUT = -1, E_FULL = -2, E_DATA = -3 };

static int need(State *s, u32 bits)
{
    while (s->bitcnt < bits) {
        if (s->pos >= s->inlen) return E_INPUT;
        s->bitbuf |= (u32)s->in[s->pos++] << s->bitcnt;
        s->bitcnt += 8;
    }
    return 0;
}

static int getbits(State *s, u32 n, u32 *v)
{
    if (n == 0) {
        *v = 0;
        return 0;
    }
    if (need(s, n)) return E_INPUT;
    *v = s->bitbuf & ((1u << n) - 1);
    s->bitbuf >>= n;
    s->bitcnt -= n;
    return 0;
}

static int put(State *s, u8 c)
{
    if (s->out->len >= s->maxout) return E_FULL;
    if (s->out->len >= s->out->cap && buf_reserve(s->out, 1)) return E_FULL;
    s->out->p[s->out->len++] = c;
    return 0;
}

static int build(Huff *h, const u8 *lengths, int n)
{
    u16 offs[16];
    int len, sym, left = 1;
    for (len = 0; len < 16; len++) h->count[len] = 0;
    for (sym = 0; sym < n; sym++) h->count[lengths[sym]]++;
    if (h->count[0] == n) return 0; /* no codes: fine until used */
    for (len = 1; len < 16; len++) {
        left <<= 1;
        left -= h->count[len];
        if (left < 0) return E_DATA; /* over-subscribed */
    }
    offs[1] = 0;
    for (len = 1; len < 15; len++) offs[len + 1] = (u16)(offs[len] + h->count[len]);
    for (sym = 0; sym < n; sym++)
        if (lengths[sym]) h->symbol[offs[lengths[sym]]++] = (u16)sym;
    return 0;
}

static int decode(State *s, const Huff *h)
{
    int code = 0, first = 0, index = 0, len;
    for (len = 1; len < 16; len++) {
        int count;
        u32 bit;
        if (getbits(s, 1, &bit)) return E_INPUT;
        code |= (int)bit;
        count = h->count[len];
        if (code - count < first) return h->symbol[index + (code - first)];
        index += count;
        first += count;
        first <<= 1;
        code <<= 1;
    }
    return E_DATA;
}

static const u16 LBASE[29] = {3, 4, 5, 6, 7, 8, 9, 10, 11, 13, 15, 17, 19, 23, 27, 31,
                              35, 43, 51, 59, 67, 83, 99, 115, 131, 163, 195, 227, 258};
static const u8 LEXTRA[29] = {0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2,
                              3, 3, 3, 3, 4, 4, 4, 4, 5, 5, 5, 5, 0};
static const u16 DBASE[30] = {1, 2, 3, 4, 5, 7, 9, 13, 17, 25, 33, 49, 65, 97, 129,
                              193, 257, 385, 513, 769, 1025, 1537, 2049, 3073, 4097,
                              6145, 8193, 12289, 16385, 24577};
static const u8 DEXTRA[30] = {0, 0, 0, 0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6,
                              6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13};

static int codes(State *s, const Huff *lit, const Huff *dist)
{
    for (;;) {
        int sym = decode(s, lit);
        if (sym < 0) return sym;
        if (sym < 256) {
            int r = put(s, (u8)sym);
            if (r) return r;
        } else if (sym == 256) {
            return 0;
        } else {
            u32 extra, len, d, i;
            int ds;
            sym -= 257;
            if (sym >= 29) return E_DATA;
            if (getbits(s, LEXTRA[sym], &extra)) return E_INPUT;
            len = LBASE[sym] + extra;
            ds = decode(s, dist);
            if (ds < 0) return ds;
            if (ds >= 30) return E_DATA;
            if (getbits(s, DEXTRA[ds], &extra)) return E_INPUT;
            d = DBASE[ds] + extra;
            if (d > s->out->len) return E_DATA;
            for (i = 0; i < len; i++) {
                int r = put(s, s->out->p[s->out->len - d]);
                if (r) return r;
            }
        }
    }
}

static int stored(State *s)
{
    u32 len, nlen;
    s->bitbuf = 0;
    s->bitcnt = 0;
    if (s->pos + 4 > s->inlen) return E_INPUT;
    len = rd_le16(s->in + s->pos);
    nlen = rd_le16(s->in + s->pos + 2);
    s->pos += 4;
    if (len != (~nlen & 0xFFFF)) return E_DATA;
    if (s->pos + len > s->inlen) return E_INPUT;
    while (len--) {
        int r = put(s, s->in[s->pos++]);
        if (r) return r;
    }
    return 0;
}

static int fixed(State *s)
{
    static Huff lit, dist;
    static int ready;
    if (!ready) {
        u8 l[288];
        int i;
        for (i = 0; i < 144; i++) l[i] = 8;
        for (; i < 256; i++) l[i] = 9;
        for (; i < 280; i++) l[i] = 7;
        for (; i < 288; i++) l[i] = 8;
        build(&lit, l, 288);
        for (i = 0; i < 30; i++) l[i] = 5;
        build(&dist, l, 30);
        ready = 1;
    }
    return codes(s, &lit, &dist);
}

static int dynamic(State *s)
{
    static const u8 ORDER[19] = {16, 17, 18, 0, 8, 7, 9, 6, 10, 5, 11, 4, 12, 3, 13, 2, 14, 1, 15};
    u8 lengths[320];
    u32 nlen, ndist, ncode, i, v;
    Huff lencode, lit, dist;
    int r;
    if (getbits(s, 5, &nlen) || getbits(s, 5, &ndist) || getbits(s, 4, &ncode)) return E_INPUT;
    nlen += 257;
    ndist += 1;
    ncode += 4;
    if (nlen > 286 || ndist > 30) return E_DATA;
    for (i = 0; i < 19; i++) lengths[ORDER[i]] = 0;
    for (i = 0; i < ncode; i++) {
        if (getbits(s, 3, &v)) return E_INPUT;
        lengths[ORDER[i]] = (u8)v;
    }
    if (build(&lencode, lengths, 19)) return E_DATA;
    for (i = 0; i < nlen + ndist;) {
        int sym = decode(s, &lencode);
        u32 rep, len = 0;
        if (sym < 0) return sym;
        if (sym < 16) {
            lengths[i++] = (u8)sym;
            continue;
        }
        if (sym == 16) {
            if (i == 0) return E_DATA;
            len = lengths[i - 1];
            if (getbits(s, 2, &rep)) return E_INPUT;
            rep += 3;
        } else if (sym == 17) {
            if (getbits(s, 3, &rep)) return E_INPUT;
            rep += 3;
        } else {
            if (getbits(s, 7, &rep)) return E_INPUT;
            rep += 11;
        }
        if (i + rep > nlen + ndist) return E_DATA;
        while (rep--) lengths[i++] = (u8)len;
    }
    if (lengths[256] == 0) return E_DATA;
    r = build(&lit, lengths, (int)nlen);
    if (r) return r;
    r = build(&dist, lengths + nlen, (int)ndist);
    if (r) return r;
    return codes(s, &lit, &dist);
}

int inflate_raw(const u8 *in, u32 inlen, Buf *out, u32 maxout, u32 *used)
{
    State s;
    u32 last, type;
    int r;
    s.in = in;
    s.inlen = inlen;
    s.pos = 0;
    s.bitbuf = 0;
    s.bitcnt = 0;
    s.out = out;
    s.maxout = maxout;
    do {
        if (getbits(&s, 1, &last) || getbits(&s, 2, &type)) {
            r = E_INPUT;
            break;
        }
        if (type == 0) r = stored(&s);
        else if (type == 1) r = fixed(&s);
        else if (type == 2) r = dynamic(&s);
        else r = E_DATA;
    } while (!r && !last);
    if (used) *used = s.pos;
    return r;
}

static u32 adler32(const u8 *p, u32 n)
{
    u32 a = 1, b = 0;
    while (n) {
        u32 k = n < 5552 ? n : 5552;
        n -= k;
        while (k--) {
            a += *p++;
            b += a;
        }
        a %= 65521;
        b %= 65521;
    }
    return (b << 16) | a;
}

int zlib_header_ok(const u8 *p, u32 n)
{
    if (n < 2) return 0;
    if ((p[0] & 0x0F) != 8 || (p[0] >> 4) > 7) return 0;
    if (p[1] & 0x20) return 0; /* preset dictionary: never used by these files */
    return ((p[0] << 8) | p[1]) % 31 == 0;
}

int inflate_zlib(const u8 *in, u32 inlen, Buf *out, u32 maxout, u32 *used)
{
    u32 start = out->len, n = 0;
    int r;
    if (!zlib_header_ok(in, inlen)) return E_DATA;
    r = inflate_raw(in + 2, inlen - 2, out, maxout, &n);
    if (used) *used = 2 + n;
    if (r) return r;
    if (2 + n + 4 > inlen) return E_INPUT;
    if (rd_be32(in + 2 + n) != adler32(out->p + start, out->len - start)) return E_DATA;
    if (used) *used = 2 + n + 4;
    return 0;
}
