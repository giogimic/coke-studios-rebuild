/* Deciding what a found file is and keeping it: the one place every find
 * passes through, whichever cache, folder or disk it came out of. */
#include "finder.h"
#include "hash.h"

/* ---- small lists ---- */

static int date_before(const Date *a, const Date *b)
{
    if (a->year != b->year) return a->year < b->year;
    if (a->month != b->month) return a->month < b->month;
    if (a->day != b->day) return a->day < b->day;
    if (a->hour != b->hour) return a->hour < b->hour;
    return a->minute < b->minute;
}

static Note *note_add(Notes *l, const char *text, const Date *d)
{
    int i;
    for (i = 0; i < l->n; i++) {
        if (!str_ieq(l->v[i].text, text)) continue;
        l->v[i].count++;
        if (d && d->year && (!l->v[i].date.year || date_before(d, &l->v[i].date))) l->v[i].date = *d;
        return &l->v[i];
    }
    if (l->n >= 5000) return NULL;
    if (l->n == l->cap) {
        int cap = l->cap ? l->cap * 2 : 32;
        Note *v = (Note *)mem_realloc(l->v, (size_t)cap * sizeof *v);
        if (!v) return NULL;
        l->v = v;
        l->cap = cap;
    }
    l->v[l->n].text = str_dup(text);
    l->v[l->n].count = 1;
    if (d) l->v[l->n].date = *d;
    else memset(&l->v[l->n].date, 0, sizeof(Date));
    return &l->v[l->n++];
}

void add_note(Scan *s, const char *text, const Date *d)
{
    note_add(&s->notes, text, d);
}

void add_named(Scan *s, const char *path, u64 size, const Date *d)
{
    char *r = redact(path), *t = fmt("%s  (%M)", r, size);
    note_add(&s->named, t, d);
    mem_free(r);
    mem_free(t);
}

static int urlchar(u8 c)
{
    return c > 32 && c < 127 && c != '"' && c != '\'' && c != '<' && c != '>' && c != '\\' &&
           c != '^' && c != '`' && c != '{' && c != '|' && c != '}';
}

/* Where the address starts in "HTTP:http://..." (Firefox's cache keys) or
 * "Visited: name@http://..." (IE's history). NULL if there is none. */
const char *url_start(const char *s)
{
    const char *p = s ? str_ifind(s, "://") : NULL, *b;
    if (!p) return NULL;
    for (b = p; b > s && ((b[-1] | 32) >= 'a' && (b[-1] | 32) <= 'z'); b--) {}
    return b == p ? NULL : b;
}

void add_url(Scan *s, const char *url, const Date *d)
{
    Str b = {0};
    /* only real addresses: a cookie's "Cookie:name@host" would carry the
     * Windows user name */
    const char *p = url_start(url);
    if (!p) return;
    /* query strings can carry session ids and screen names: never keep them */
    for (; *p && *p != '?' && *p != '#' && *p != '&' && *p != ';' && urlchar((u8)*p); p++) str_addc(&b, *p);
    if (b.len >= 10 && coke_url(b.s) && !str_ifind(b.s, "pass") && !str_ifind(b.s, "pwd"))
        note_add(&s->urls, b.s, d);
    str_free(&b);
}

static int hostchar(u8 c)
{
    return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-';
}

int url_at(Scan *s, const u8 *p, u32 i, u32 n, const Date *d)
{
    u32 j = i, start, e;
    int k = match_host_at(p + i, n - i);
    Str b = {0};
    if (!k) return 0;
    if (j > 0 && hostchar(p[j - 1]) && p[j - 1] != '.') return 0; /* inside a longer word */
    while (j > 0 && hostchar(p[j - 1])) j--;
    start = j;
    if (j >= 3 && p[j - 1] == '/' && p[j - 2] == '/' && p[j - 3] == ':') {
        start = j - 3;
        while (start > 0 && start > j - 12 && ((p[start - 1] | 32) >= 'a' && (p[start - 1] | 32) <= 'z')) start--;
    } else {
        str_add(&b, "http://");
    }
    for (e = i + (u32)k; e < n && e - start < 400 && urlchar(p[e]); e++) {}
    str_addn(&b, (const char *)p + start, e - start);
    add_url(s, b.s, d);
    str_free(&b);
    return (int)(e - i);
}

void urls_in(Scan *s, const u8 *p, u32 n, const Date *d)
{
    u32 i;
    for (i = 0; i < n; i++) {
        u8 c = (u8)(p[i] | 32);
        if (c == 'c' || c == 'm' || c == 's' || c == 'a' || c == 'f') {
            int skip = url_at(s, p, i, n, d);
            if (skip > 0) i += (u32)skip - 1;
        }
    }
}

/* ---- paths ---- */

static int seg_is(const char *seg, size_t len, const char *word)
{
    return seg && len == str_len(word) && str_istarts(seg, word);
}

/* Takes the user's name out of a path: the folder under Documents and
 * Settings, Users, home or Windows\Profiles, and anything before an @ (IE
 * names cookie and history entries user@site). */
char *redact(const char *path)
{
    static const char *const PARENTS[] = {"documents and settings", "users", "home", 0};
    static const char *const KEEP[] = {"all users", "default user", "default", "public", "shared",
                                       "localservice", "networkservice", "default.migrated", 0};
    Str out = {0};
    const char *p = path, *prev = NULL, *prev2 = NULL;
    size_t prevlen = 0, prev2len = 0;
    while (*p) {
        const char *seg = p, *at;
        size_t len, k;
        int hide = 0, i;
        while (*p && *p != '\\' && *p != '/') p++;
        len = (size_t)(p - seg);
        if (prev && len) {
            for (i = 0; PARENTS[i]; i++)
                if (seg_is(prev, prevlen, PARENTS[i])) hide = 1;
            if (seg_is(prev, prevlen, "profiles") && (seg_is(prev2, prev2len, "windows") || seg_is(prev2, prev2len, "winnt")))
                hide = 1;
            for (i = 0; hide && KEEP[i]; i++)
                if (seg_is(seg, len, KEEP[i])) hide = 0;
        }
        for (at = NULL, k = 0; k < len; k++)
            if (seg[k] == '@') at = seg + k;
        if (hide) {
            str_add(&out, "[user]");
        } else if (at) {
            str_add(&out, "[user]");
            str_addn(&out, at, len - (size_t)(at - seg));
        } else {
            str_addn(&out, seg, len);
        }
        if (*p) str_addc(&out, *p++);
        prev2 = prev;
        prev2len = prevlen;
        prev = seg;
        prevlen = len;
    }
    return out.s ? out.s : str_dup("");
}

/* ---- the progress line ---- */

void status(Scan *s, const char *what, const char *path)
{
    u32 now = ms_now();
    Str b = {0};
    size_t n, max = 76;
    if (!con_interactive()) { /* to a log: a plain line now and then */
        if (now - s->status_ms >= 30000) {
            s->status_ms = now;
            say("  ... (%N checked so far) %s %s\n", s->files_checked, what, path);
        }
        return;
    }
    if (now - s->status_ms < 200) return;
    s->status_ms = now;
    str_add(&b, "\r  ");
    str_add(&b, what);
    str_addc(&b, ' ');
    n = str_len(path);
    if (b.len + n > max) {
        str_add(&b, "...");
        str_add(&b, path + n - (max - b.len));
    } else {
        str_add(&b, path);
    }
    while (b.len < max + 1) str_addc(&b, ' ');
    con_write(b.s);
    str_free(&b);
    s->status_shown = 1;
}

void status_clear(Scan *s)
{
    if (!s->status_shown) return;
    con_write("\r                                                                             \r");
    s->status_shown = 0;
    s->status_ms = 0;
}

/* ---- files a cache index already dealt with ---- */

static u32 path_hash(const char *p)
{
    u32 h = 2166136261u;
    for (; *p; p++) h = (h ^ (u32)lower((u8)*p)) * 16777619u;
    return h ? h : 1;
}

void mark_done(Scan *s, const char *path)
{
    u32 i, h = path_hash(path);
    if ((u32)s->ndone * 2 >= (u32)s->capdone) {
        int cap = s->capdone ? s->capdone * 2 : 256;
        char **old = s->done;
        int oldcap = s->capdone, k;
        s->done = (char **)mem_alloc((size_t)cap * sizeof(char *));
        if (!s->done) {
            s->done = old;
            return;
        }
        s->capdone = cap;
        for (k = 0; k < oldcap; k++) {
            if (!old[k]) continue;
            for (i = path_hash(old[k]) & (u32)(cap - 1); s->done[i]; i = (i + 1) & (u32)(cap - 1)) {}
            s->done[i] = old[k];
        }
        mem_free(old);
    }
    for (i = h & (u32)(s->capdone - 1); s->done[i]; i = (i + 1) & (u32)(s->capdone - 1))
        if (str_ieq(s->done[i], path)) return;
    s->done[i] = str_dup(path);
    s->ndone++;
}

int is_done(Scan *s, const char *path)
{
    u32 i;
    if (!s->capdone) return 0;
    for (i = path_hash(path) & (u32)(s->capdone - 1); s->done[i]; i = (i + 1) & (u32)(s->capdone - 1))
        if (str_ieq(s->done[i], path)) return 1;
    return 0;
}

/* ---- dates ---- */

void date_from_unix(u32 t, Date *d)
{
    int z = (int)(t / 86400) + 719468, era, doe, yoe, y, doy, mp, m;
    u32 rem = t % 86400;
    era = z / 146097;
    doe = z - era * 146097;
    yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    y = yoe + era * 400;
    doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    mp = (5 * doy + 2) / 153;
    m = mp < 10 ? mp + 3 : mp - 9;
    d->day = (u8)(doy - (153 * mp + 2) / 5 + 1);
    d->month = (u8)m;
    d->year = (u16)(y + (m <= 2));
    d->hour = (u8)(rem / 3600);
    d->minute = (u8)(rem / 60 % 60);
    d->second = (u8)(rem % 60);
}

/* ---- the finds ---- */

static const char *what_by_ext(const char *name)
{
    const char *e = path_ext(name);
    if (str_ieq(e, "gif") || str_ieq(e, "jpg") || str_ieq(e, "jpeg") || str_ieq(e, "png") || str_ieq(e, "bmp"))
        return "Picture";
    if (str_ieq(e, "htm") || str_ieq(e, "html") || str_ieq(e, "jsp") || str_ieq(e, "asp") || str_ieq(e, "php"))
        return "Web page";
    if (str_ieq(e, "txt")) return "Text file";
    if (str_ieq(e, "xml")) return "XML file";
    if (str_ieq(e, "mp3") || str_ieq(e, "wav") || str_ieq(e, "swa")) return "Sound";
    if (str_ieq(e, "js") || str_ieq(e, "css")) return "Web page part";
    return "File";
}

static char *name_from_url(const char *url)
{
    const char *p = str_ifind(url, "://"), *base;
    Str b = {0};
    p = p ? p + 3 : url;
    base = p;
    for (; *p && *p != '?' && *p != '#'; p++)
        if (*p == '/') base = p + 1;
    for (p = base; *p && *p != '?' && *p != '#'; p++) {
        if (*p == '%' && p[1] && p[2]) {
            int hi = lower((u8)p[1]), lo = lower((u8)p[2]), v;
            hi = hi <= '9' ? hi - '0' : hi - 'a' + 10;
            lo = lo <= '9' ? lo - '0' : lo - 'a' + 10;
            v = hi * 16 + lo;
            if (hi >= 0 && hi < 16 && lo >= 0 && lo < 16 && v >= 32) {
                str_addc(&b, (char)v);
                p += 2;
                continue;
            }
        }
        str_addc(&b, *p);
    }
    if (!b.len) {
        char host[128];
        url_host(url, host, sizeof host);
        str_add(&b, host);
        str_add(&b, ".html");
    }
    return b.s;
}

static char *clean_name(const char *name, const char *ext)
{
    Str b = {0};
    const char *p, *have;
    for (p = name; *p && b.len < 120; p++) {
        u8 c = (u8)*p;
        str_addc(&b, (c < 32 || c == '\\' || c == '/' || c == ':' || c == '*' || c == '?' || c == '"' ||
                      c == '<' || c == '>' || c == '|')
                         ? '_'
                         : (char)c);
    }
    if (!b.len || str_eq(b.s, ".") || str_eq(b.s, "..")) {
        str_free(&b);
        str_add(&b, "unnamed");
    }
    have = path_ext(b.s);
    if (ext && !str_ieq(have, ext)) {
        int director = str_ieq(ext, "cct") || str_ieq(ext, "dcr") || str_ieq(ext, "dir") || str_ieq(ext, "cst");
        int fine = director && (str_ieq(have, "cct") || str_ieq(have, "dcr") || str_ieq(have, "dir") ||
                                str_ieq(have, "cst") || str_ieq(have, "cxt") || str_ieq(have, "dxr"));
        if (!fine) {
            str_addc(&b, '.');
            str_add(&b, ext);
        }
    }
    return b.s;
}

static char *save(Scan *s, const char *folder, const char *name, const u8 *data, u32 len)
{
    char *dir, *rel = NULL, *full = NULL, *stem;
    const char *ext = path_ext(name);
    int i;
    PFile *f;
    if (ensure_out_dir(s)) return NULL;
    dir = fmt("%s%cfiles", s->out_dir, path_sep());
    dir_make(dir);
    mem_free(dir);
    dir = fmt("%s%cfiles%c%s", s->out_dir, path_sep(), path_sep(), folder);
    dir_make(dir);
    stem = str_ndup(name, str_len(name) - (*ext ? str_len(ext) + 1 : 0));
    for (i = 1; i < 1000; i++) {
        char *n = i == 1 ? str_dup(name) : fmt("%s~%d%s%s", stem, i, *ext ? "." : "", ext);
        mem_free(full);
        full = path_join(dir, n);
        if (path_kind(full) < 0) {
            rel = fmt("files/%s/%s", folder, n);
            mem_free(n);
            break;
        }
        mem_free(n);
    }
    mem_free(dir);
    mem_free(stem);
    if (!rel) {
        mem_free(full);
        return NULL;
    }
    f = f_open_write(full);
    if (!f || f_write(f, data, len)) {
        if (f) f_close(f);
        mem_free(full);
        mem_free(rel);
        return NULL;
    }
    f_close(f);
    mem_free(full);
    return rel;
}

static Finding *new_finding(Scan *s)
{
    if (s->nf == s->capf) {
        int cap = s->capf ? s->capf * 2 : 64;
        Finding *v = (Finding *)mem_realloc(s->f, (size_t)cap * sizeof *v);
        if (!v) return NULL;
        s->f = v;
        s->capf = cap;
    }
    memset(&s->f[s->nf], 0, sizeof(Finding));
    return &s->f[s->nf++];
}

static int is_v1_era(const Date *a, const Date *b)
{
    return (a->year >= 2002 && a->year <= 2004) || (b->year >= 2002 && b->year <= 2004);
}

void consider(Scan *s, const Blob *b)
{
    Sniff sn;
    Exam x;
    Markers m;
    Finding *f;
    u8 digest[20];
    char hex[41], host[128];
    const char *known, *ident = NULL, *ident_wanted = NULL;
    int kind, from_site, cat, i;
    u32 len;
    Str why = {0};
    char *name, *base;

    memset(&m, 0, sizeof m);
    kind = sniff(b->data, b->len, &sn, b->carved) ? sn.kind : K_NONE;
    from_site = coke_url(b->url);
    if (kind == K_NONE) {
        if (!from_site && !(b->text_hint && known_text_marker(b->data, b->len))) return;
        memset(&x, 0, sizeof x);
        x.length = b->len;
    } else if (!examine(b->data, b->len, &sn, b->carved, &x)) {
        exam_free(&x);
        return;
    }
    len = x.length;
    sha1(b->data, len, digest);
    sha1_hex(digest, hex);

    for (i = 0; i < s->nf; i++) {
        if (s->f[i].size != len || !str_eq(s->f[i].sha1, hex)) continue;
        s->f[i].copies++;
        if (b->url && !s->f[i].url) s->f[i].url = str_dup(b->url);
        if (!s->f[i].server_date.year && b->server_date.year) s->f[i].server_date = b->server_date;
        exam_free(&x);
        return;
    }

    known = known_name(hex, len);
    find_markers(b->data, len, &m);
    if (x.text.len) find_markers(x.text.p, x.text.len, &m);
    /* the archive's own files count whatever words they contain: many of
     * the website's movies and the mixer's samples have none; so do a few
     * lost ones, which their names give away */
    if (known || from_site || m.strong || (kind == K_NONE && b->text_hint)) cat = CAT_COKE;
    else if (name_identifies(b->name ? b->name : path_base(b->path)) || name_identifies(b->url)) cat = CAT_COKE;
    else if (kind == K_DIRECTOR) cat = m.weak ? CAT_COKE : CAT_OTHER;
    else if (kind == K_SWF && m.weak) cat = CAT_COKE;
    else cat = 0;

    if (s->verbose) {
        status_clear(s);
        say("%s  %s  %s  [%s]\n", cat == CAT_COKE ? "COKE " : cat == CAT_OTHER ? "OTHER" : "drop ",
            kind == K_DIRECTOR ? sn.what : kind == K_SWF ? "swf" : "text", b->path, m.words.s ? m.words.s : "");
    }
    if (!cat) {
        exam_free(&x);
        markers_free(&m);
        return;
    }

    if (from_site) {
        url_host(b->url, host, sizeof host);
        str_fmt(&why, "came from %s", host);
    }
    if (m.words.len) {
        if (why.len) str_add(&why, "; ");
        str_add(&why, "contains ");
        str_add(&why, m.words.s);
    }
    if (!why.len)
        str_add(&why, known ? "it is in the archive" : cat == CAT_OTHER ? "a Shockwave file with no Coke words in it" : "its name");

    /* what the file's own contents say it is - the only clue for something
     * carved out of disk space, which has no name and no address. A file
     * with a real name (from a folder or a cache address) is left as it is.
     * Rooms and the big named files are Director casts or text; a Flash movie
     * is named from its address, so it is not run through this. */
    if (b->carved && kind != K_SWF)
        ident = identify_content(b->data, len, x.text.p, x.text.len, &ident_wanted);

    /* a carved find gets its name from what it turned out to be, so the
     * zip reads "wal-mart-music-downloads_at_131072.cct" and not "disk" */
    if (b->carved && ident) {
        char *slug = label_slug(ident);
        base = fmt("%s_at_%U", slug, b->offset);
        mem_free(slug);
    } else {
        base = b->url ? name_from_url(b->url) : str_dup(b->name ? b->name : path_base(b->path));
    }
    name = clean_name(base, kind == K_NONE ? NULL : sn.ext);
    mem_free(base);

    f = new_finding(s);
    if (!f) {
        mem_free(name);
        str_free(&why);
        exam_free(&x);
        markers_free(&m);
        return;
    }
    f->cat = cat;
    f->name = name;
    f->what = kind == K_NONE ? what_by_ext(name) : sn.what;
    f->why = why.s;
    f->identified = ident;
    f->known = known;
    f->size = len;
    f->damaged = x.damaged;
    f->carved = b->carved;
    f->file_date = b->file_date;
    f->server_date = b->server_date;
    memcpy(f->sha1, hex, sizeof hex);
    if (b->url) f->url = str_dup(b->url);
    if (!known) {
        f->wanted = wanted_label(name, b->url);
        /* a carved cast that turned out to be a lost room is named by its
         * contents, not its address, so the content check supplies the want */
        if (!f->wanted) f->wanted = ident_wanted;
        /* the archive's oldest client is from March 2006 and its website
         * movies from 2006-2007, so anything older is a lost version */
        if (!f->wanted && cat == CAT_COKE && is_v1_era(&b->file_date, &b->server_date))
            f->wanted = kind == K_DIRECTOR
                            ? "a 2002-2004 file: version 1, the 2.0 beta or the Shark Tale edition, none of which survive"
                            : "a 2002-2004 file, older than anything in the archive";
    }
    {
        char *r = redact(b->path);
        if (b->carved) f->source = fmt("%s, at byte %U", r, b->offset);
        else f->source = str_dup(r);
        mem_free(r);
    }
    if (!known && !s->dry_run) f->saved = save(s, cat == CAT_COKE ? "coke" : "other-shockwave", name, b->data, len);

    if (!s->verbose) {
        status_clear(s);
        if (known) say("  found  %s  (%s - already in the archive)\n", name, f->what);
        else if (cat == CAT_COKE) say("  FOUND  %s  (%s) - NEW!%s%s\n", name, f->what, f->wanted ? "  Wanted: " : "", f->wanted ? f->wanted : "");
        else say("  found  %s  (%s, maybe Coke Studios)\n", name, f->what);
    }
    exam_free(&x);
    markers_free(&m);
}
