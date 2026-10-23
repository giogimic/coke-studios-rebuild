/* A minimal zip writer: stored entries only. Casts and compressed Flash are
 * already zlib inside, so compressing again would buy almost nothing, and
 * "stored" is the one method every unzip tool since 1993 can read. */
#include "zip.h"
#include "hash.h"

typedef struct {
    char *name;
    u32 crc, size, offset;
    u16 time, date;
} Entry;

struct ZipWriter {
    PFile *f;
    u32 offset;
    Entry *e;
    int n, cap;
    int failed;
};

static void dos_time(const Date *d, u16 *t, u16 *dt)
{
    Date now;
    if (!d || !d->year) {
        date_now(&now);
        d = &now;
    }
    *t = (u16)((d->hour << 11) | (d->minute << 5) | (d->second / 2));
    *dt = (u16)(((d->year < 1980 ? 0 : d->year - 1980) << 9) | (d->month << 5) | d->day);
}

static void put(ZipWriter *z, const void *p, u32 n)
{
    if (z->failed) return;
    if (f_write(z->f, p, n)) z->failed = 1;
    z->offset += n;
}

ZipWriter *zip_create(const char *path)
{
    ZipWriter *z = (ZipWriter *)mem_alloc(sizeof *z);
    if (!z) return NULL;
    z->f = f_open_write(path);
    if (!z->f) {
        mem_free(z);
        return NULL;
    }
    return z;
}

int zip_add(ZipWriter *z, const char *name, const u8 *data, u32 len, const Date *d)
{
    u8 h[30];
    Entry *e;
    u32 nlen = (u32)str_len(name);
    if (z->n == z->cap) {
        int cap = z->cap ? z->cap * 2 : 64;
        Entry *ne = (Entry *)mem_realloc(z->e, (size_t)cap * sizeof *ne);
        if (!ne) return -1;
        z->e = ne;
        z->cap = cap;
    }
    if ((u64)z->offset + len + nlen + 30 > 0xF0000000u) return -1; /* no zip64 */
    e = &z->e[z->n++];
    e->name = str_dup(name);
    e->crc = crc32(0, data, len);
    e->size = len;
    e->offset = z->offset;
    dos_time(d, &e->time, &e->date);
    memset(h, 0, sizeof h);
    wr_le32(h, 0x04034b50);
    wr_le16(h + 4, 10);      /* version needed: 1.0 */
    wr_le16(h + 6, 1 << 11); /* names are UTF-8 */
    wr_le16(h + 8, 0);       /* stored */
    wr_le16(h + 10, e->time);
    wr_le16(h + 12, e->date);
    wr_le32(h + 14, e->crc);
    wr_le32(h + 18, len);
    wr_le32(h + 22, len);
    wr_le16(h + 26, (u16)nlen);
    put(z, h, 30);
    put(z, name, nlen);
    put(z, data, len);
    return z->failed ? -1 : 0;
}

int zip_close(ZipWriter *z)
{
    u32 cd_start = z->offset, i;
    u8 h[46], end[22];
    int failed;
    for (i = 0; i < (u32)z->n; i++) {
        Entry *e = &z->e[i];
        u32 nlen = (u32)str_len(e->name);
        memset(h, 0, sizeof h);
        wr_le32(h, 0x02014b50);
        wr_le16(h + 4, 20);
        wr_le16(h + 6, 10);
        wr_le16(h + 8, 1 << 11);
        wr_le16(h + 12, e->time);
        wr_le16(h + 14, e->date);
        wr_le32(h + 16, e->crc);
        wr_le32(h + 20, e->size);
        wr_le32(h + 24, e->size);
        wr_le16(h + 28, (u16)nlen);
        wr_le32(h + 42, e->offset);
        put(z, h, 46);
        put(z, e->name, nlen);
        mem_free(e->name);
    }
    memset(end, 0, sizeof end);
    wr_le32(end, 0x06054b50);
    wr_le16(end + 8, (u16)z->n);
    wr_le16(end + 10, (u16)z->n);
    wr_le32(end + 12, z->offset - cd_start);
    wr_le32(end + 16, cd_start);
    put(z, end, 22);
    f_close(z->f);
    failed = z->failed;
    mem_free(z->e);
    mem_free(z);
    return failed ? -1 : 0;
}
