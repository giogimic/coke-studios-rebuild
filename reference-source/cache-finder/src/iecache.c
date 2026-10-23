/* Internet Explorer's cache index, index.dat ("Client UrlCache MMF Ver
 * 5.2", IE 5 to 9).
 *
 * This is the most important file on an old Windows PC for this search:
 * Shockwave in Internet Explorer downloaded every cast through IE, so a
 * cast sits in Temporary Internet Files\Content.IE5\<8 letters>\ under its
 * own name plus "[1]" - exactly the names the archive's casts still carry -
 * and index.dat says which address each file came from and when the server
 * last changed it. That date is the closest thing to a version number a
 * cast has. History and cookie folders keep an index.dat of the same
 * format, which is where old addresses survive after the files are gone.
 *
 * Layout (libmsiecf's documentation of the format): directory count at
 * 0x48, 12-byte directory entries from 0x4C (name at +4, 8 letters);
 * records in 128-byte blocks from 0x4000. A "URL " record has the server's
 * modification time at +0x08, last access at +0x10, the address's offset at
 * +0x34, the directory index at +0x38 and the file name's offset at +0x3C. */
#include "finder.h"

static char *field(const u8 *r, u32 rl, u32 off)
{
    u32 e;
    if (off < 0x40 || off >= rl) return NULL;
    for (e = off; e < rl && r[e]; e++)
        if (r[e] < 32 || r[e] > 126) return NULL;
    if (e == off) return NULL;
    return str_ndup((const char *)r + off, e - off);
}

static void when(const u8 *p, Date *d)
{
    u64 ft = rd_le64(p);
    memset(d, 0, sizeof *d);
    if (!ft) return;
    filetime_to_date(ft, d);
    if (d->year < 1995 || d->year > 2040) memset(d, 0, sizeof *d);
}

/* Worth reading even from another site: a CDN may have served the casts. */
static int worth(const char *url, const char *file)
{
    const char *e = path_ext(file);
    return coke_url(url) || str_ieq(e, "cct") || str_ieq(e, "dcr") || str_ieq(e, "dir") ||
           str_ieq(e, "cst") || str_ieq(e, "cxt") || str_ieq(e, "dxr") || str_ieq(e, "swf") ||
           text_name_wanted(file);
}

void ie_index(Scan *s, const char *dir)
{
    char *path = path_join(dir, "index.dat");
    char names[64][9];
    Buf b = {0};
    u32 ndirs = 0, i, off;
    if (read_file(path, 64u << 20, &b, NULL)) {
        mem_free(path);
        return;
    }
    mem_free(path);
    if (b.len < 0x4000 || memcmp(b.p, "Client UrlCache MMF Ver 5.2", 27)) {
        /* IE 4's 4.7 layout, or some other index.dat: just look for addresses */
        urls_in(s, b.p, b.len, NULL);
        buf_free(&b);
        return;
    }
    ndirs = rd_le32(b.p + 0x48);
    if (ndirs > 64 || 0x4C + ndirs * 12 > b.len) ndirs = 0;
    for (i = 0; i < ndirs; i++) {
        memcpy(names[i], b.p + 0x4C + i * 12 + 4, 8);
        names[i][8] = 0;
    }
    for (off = 0x4000; off + 0x80 <= b.len;) {
        const u8 *r = b.p + off;
        u32 step = 0x80;
        if (!memcmp(r, "URL ", 4)) {
            u32 nb = rd_le32(r + 4);
            if (nb >= 1 && nb <= 4096) {
                u32 rl = nb << 7;
                char *url, *file, *http;
                Date modified, accessed;
                if (off + rl > b.len) rl = b.len - off;
                step = rl;
                url = field(r, rl, rd_le32(r + 0x34));
                file = field(r, rl, rd_le32(r + 0x3C));
                when(r + 0x08, &modified);
                when(r + 0x10, &accessed);
                http = (char *)url_start(url);
                if (http && coke_url(http)) add_url(s, http, accessed.year ? &accessed : &modified);
                if (http && file && r[0x38] < ndirs && worth(http, file)) {
                    char *sub = path_join(dir, names[r[0x38]]), *fp = path_join(sub, file);
                    Buf data = {0};
                    if (!is_done(s, fp) && path_kind(fp) == 0 && !read_file(fp, MAX_FILE, &data, NULL)) {
                        Blob bl;
                        memset(&bl, 0, sizeof bl);
                        bl.data = data.p;
                        bl.len = data.len;
                        bl.path = fp;
                        bl.name = file;
                        bl.url = http;
                        bl.file_date = accessed;
                        bl.server_date = modified;
                        bl.text_hint = text_name_wanted(file);
                        consider(s, &bl);
                        mark_done(s, fp);
                    }
                    buf_free(&data);
                    mem_free(sub);
                    mem_free(fp);
                }
                mem_free(url);
                mem_free(file);
            }
        }
        off += step;
    }
    buf_free(&b);
}
