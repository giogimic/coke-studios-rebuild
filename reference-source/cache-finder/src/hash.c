/* SHA-1, to recognise files the archive already holds, and CRC-32, which
 * the zip format requires. */
#include "hash.h"

#define ROL(x, n) (((x) << (n)) | ((x) >> (32 - (n))))

static void sha1_block(u32 h[5], const u8 *p)
{
    u32 w[80], a, b, c, d, e, t;
    int i;
    for (i = 0; i < 16; i++) w[i] = rd_be32(p + 4 * i);
    for (; i < 80; i++) w[i] = ROL(w[i - 3] ^ w[i - 8] ^ w[i - 14] ^ w[i - 16], 1);
    a = h[0], b = h[1], c = h[2], d = h[3], e = h[4];
    for (i = 0; i < 80; i++) {
        u32 f, k;
        if (i < 20) f = (b & c) | (~b & d), k = 0x5A827999;
        else if (i < 40) f = b ^ c ^ d, k = 0x6ED9EBA1;
        else if (i < 60) f = (b & c) | (b & d) | (c & d), k = 0x8F1BBCDC;
        else f = b ^ c ^ d, k = 0xCA62C1D6;
        t = ROL(a, 5) + f + e + k + w[i];
        e = d, d = c, c = ROL(b, 30), b = a, a = t;
    }
    h[0] += a, h[1] += b, h[2] += c, h[3] += d, h[4] += e;
}

void sha1(const u8 *data, u32 len, u8 out[20])
{
    u32 h[5] = {0x67452301, 0xEFCDAB89, 0x98BADCFE, 0x10325476, 0xC3D2E1F0};
    u8 tail[128];
    u32 i, rest, tl;
    u64 bits = (u64)len << 3;
    for (i = 0; i + 64 <= len; i += 64) sha1_block(h, data + i);
    rest = len - i;
    memset(tail, 0, sizeof tail);
    memcpy(tail, data + i, rest);
    tail[rest] = 0x80;
    tl = rest + 1 + 8 <= 64 ? 64 : 128;
    tail[tl - 8] = (u8)(bits >> 56);
    tail[tl - 7] = (u8)(bits >> 48);
    tail[tl - 6] = (u8)(bits >> 40);
    tail[tl - 5] = (u8)(bits >> 32);
    tail[tl - 4] = (u8)(bits >> 24);
    tail[tl - 3] = (u8)(bits >> 16);
    tail[tl - 2] = (u8)(bits >> 8);
    tail[tl - 1] = (u8)bits;
    sha1_block(h, tail);
    if (tl == 128) sha1_block(h, tail + 64);
    for (i = 0; i < 5; i++) {
        out[4 * i] = (u8)(h[i] >> 24);
        out[4 * i + 1] = (u8)(h[i] >> 16);
        out[4 * i + 2] = (u8)(h[i] >> 8);
        out[4 * i + 3] = (u8)h[i];
    }
}

void sha1_hex(const u8 digest[20], char out[41])
{
    static const char HEX[] = "0123456789abcdef";
    int i;
    for (i = 0; i < 20; i++) {
        out[2 * i] = HEX[digest[i] >> 4];
        out[2 * i + 1] = HEX[digest[i] & 15];
    }
    out[40] = 0;
}

u32 crc32(u32 crc, const u8 *p, u32 n)
{
    static u32 table[256];
    static int ready;
    if (!ready) {
        u32 i, j, c;
        for (i = 0; i < 256; i++) {
            for (c = i, j = 0; j < 8; j++) c = (c & 1) ? 0xEDB88320u ^ (c >> 1) : c >> 1;
            table[i] = c;
        }
        ready = 1;
    }
    crc = ~crc;
    while (n--) crc = table[(crc ^ *p++) & 0xFF] ^ (crc >> 8);
    return ~crc;
}
