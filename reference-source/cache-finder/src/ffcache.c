/* Firefox's (and Mozilla's, and Netscape 7's) disk cache, version 1 - the
 * format from Firefox 1.0 to 31, so every Firefox of the game's lifetime.
 *
 * _CACHE_MAP_ is a table of 16-byte records - hash, eviction rank, data
 * location, metadata location - all big-endian. A location's top bit says
 * it is in use; bits 28-29 pick where: 0 a file of its own named
 * <hash><d|m><generation> (later versions put it in two levels of
 * subfolders taken from the name), 1-3 a run of blocks in _CACHE_001_ to
 * _CACHE_003_ (block number in the low 24 bits, blocks used less one in bits
 * 24-25). A metadata entry is nine big-endian words - version, its own
 * location, fetch count, last fetched, last modified, expiry, data size, key
 * size, metadata size - then the key, which is the address.
 *
 * Block sizes and the size of each block file's allocation bitmap changed
 * between versions, so each combination is tried and one is believed only
 * when the entry found there names its own location correctly. */
#include "finder.h"

typedef struct {
    const char *dir;
    PFile *block[4];
    int bshift[4]; /* log2 of the block size that checked out, 0 = unknown */
    u32 bitmap[4];
} Cache;

static const int SHIFTS[4][3] = {{0}, {8, 9, 10}, {10, 0}, {12, 0}};
static const u32 BITMAPS[] = {4096, 8192, 16384, 32768, 65536, 131072, 0};

static u64 block_off(u32 bitmap, u32 block, int shift)
{
    u64 b = block;
    switch (shift) { /* constant shifts: no 64-bit runtime helper needed */
    case 8: return bitmap + (b << 8);
    case 9: return bitmap + (b << 9);
    case 10: return bitmap + (b << 10);
    default: return bitmap + (b << 12);
    }
}

static int read_blocks(Cache *c, int sel, u32 loc, int shift, u32 bitmap, Buf *out)
{
    u32 count = ((loc >> 24) & 3) + 1, got = 0, want = count << shift;
    if (!c->block[sel]) {
        char name[16], *p;
        memcpy(name, "_CACHE_00x_", 12);
        name[9] = (char)('0' + sel);
        p = path_join(c->dir, name);
        c->block[sel] = f_open_read(p);
        mem_free(p);
        if (!c->block[sel]) return -1;
    }
    out->len = 0;
    if (buf_reserve(out, want)) return -1;
    if (f_read_at(c->block[sel], block_off(bitmap, loc & 0x00FFFFFF, shift), out->p, want, &got) || got == 0)
        return -1;
    out->len = got;
    return 0;
}

static int read_separate(Cache *c, u32 hash, u32 loc, int meta, Buf *out)
{
    char name[16], *p, *q, *r;
    const char *HEX = "0123456789ABCDEF";
    int i;
    u32 gen = loc & 0xFF;
    for (i = 0; i < 8; i++) name[i] = HEX[(hash >> (28 - 4 * i)) & 15];
    name[8] = meta ? 'm' : 'd';
    name[9] = HEX[gen >> 4];
    name[10] = HEX[gen & 15];
    name[11] = 0;
    p = path_join(c->dir, name);
    if (!read_file(p, MAX_FILE, out, NULL)) {
        mem_free(p);
        return 0;
    }
    mem_free(p);
    /* Cache/A/BC/DEFGHd01 */
    {
        char d1[2] = {name[0], 0}, d2[3] = {name[1], name[2], 0};
        p = path_join(c->dir, d1);
        q = path_join(p, d2);
        r = path_join(q, name + 3);
        i = read_file(r, MAX_FILE, out, NULL);
        mem_free(p);
        mem_free(q);
        mem_free(r);
    }
    return i;
}

static int entry_ok(const Buf *b, u32 loc)
{
    u32 keysize;
    if (b->len < 40) return 0;
    keysize = rd_be32(b->p + 28);
    return rd_be32(b->p + 4) == loc && keysize > 0 && keysize < 65536 && 36 + keysize <= b->len;
}

static int read_meta(Cache *c, u32 hash, u32 loc, Buf *out)
{
    int sel = (int)((loc >> 28) & 3), i, j;
    if (sel == 0) return read_separate(c, hash, loc, 1, out) || !entry_ok(out, loc) ? -1 : 0;
    if (c->bshift[sel]) {
        if (read_blocks(c, sel, loc, c->bshift[sel], c->bitmap[sel], out)) return -1;
        return entry_ok(out, loc) ? 0 : -1;
    }
    for (i = 0; i < 3 && SHIFTS[sel][i]; i++)
        for (j = 0; BITMAPS[j]; j++) {
            if (read_blocks(c, sel, loc, SHIFTS[sel][i], BITMAPS[j], out)) continue;
            if (entry_ok(out, loc)) {
                c->bshift[sel] = SHIFTS[sel][i];
                c->bitmap[sel] = BITMAPS[j];
                return 0;
            }
        }
    return -1;
}

static int read_data(Cache *c, u32 hash, u32 loc, u32 size, Buf *out)
{
    int sel = (int)((loc >> 28) & 3);
    if (sel == 0) {
        if (read_separate(c, hash, loc, 0, out)) return -1;
    } else {
        if (!c->bshift[sel]) { /* never confirmed by an entry: the original layout */
            c->bshift[sel] = SHIFTS[sel][0];
            c->bitmap[sel] = 4096;
        }
        if (read_blocks(c, sel, loc, c->bshift[sel], c->bitmap[sel], out)) return -1;
    }
    if (out->len > size) out->len = size;
    return 0;
}

void ff_cache(Scan *s, const char *dir)
{
    Cache c;
    Buf map = {0}, meta = {0}, data = {0};
    char *path = path_join(dir, "_CACHE_MAP_");
    u32 off;
    int i;
    memset(&c, 0, sizeof c);
    c.dir = dir;
    if (read_file(path, 32u << 20, &map, NULL)) {
        mem_free(path);
        return;
    }
    mem_free(path);
    for (off = 0; off + 16 <= map.len; off += 16) {
        u32 hash = rd_be32(map.p + off), dloc = rd_be32(map.p + off + 8), mloc = rd_be32(map.p + off + 12);
        u32 keysize, datasize;
        char *key, *http;
        Date fetched, modified;
        if (!(mloc & 0x80000000u) || hash == 0) continue;
        if (read_meta(&c, hash, mloc, &meta)) continue;
        keysize = rd_be32(meta.p + 28);
        datasize = rd_be32(meta.p + 24);
        key = str_ndup((const char *)meta.p + 36, keysize);
        http = (char *)url_start(key);
        memset(&fetched, 0, sizeof fetched);
        memset(&modified, 0, sizeof modified);
        if (rd_be32(meta.p + 12)) date_from_unix(rd_be32(meta.p + 12), &fetched);
        if (rd_be32(meta.p + 16)) date_from_unix(rd_be32(meta.p + 16), &modified);
        if (http && coke_url(http)) {
            add_url(s, http, &fetched);
            if ((dloc & 0x80000000u) && datasize > 0 && datasize <= MAX_FILE &&
                !read_data(&c, hash, dloc, datasize, &data)) {
                Blob bl;
                memset(&bl, 0, sizeof bl);
                bl.data = data.p;
                bl.len = data.len;
                bl.path = dir;
                bl.url = http;
                bl.file_date = fetched;
                bl.server_date = modified;
                consider(s, &bl);
            }
        }
        mem_free(key);
    }
    for (i = 0; i < 4; i++)
        if (c.block[i]) f_close(c.block[i]);
    buf_free(&map);
    buf_free(&meta);
    buf_free(&data);
}
