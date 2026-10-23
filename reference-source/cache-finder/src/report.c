/* The results: a folder with the new files and report.txt, and the same
 * packed into one zip for sending. Also the maintainer's --make-known,
 * which fingerprints the project's archive into known.c. */
#include "finder.h"
#include "hash.h"
#include "inflate.h"
#include "zip.h"

int ensure_out_dir(Scan *s)
{
    char *dir;
    if (s->out_dir) return 0;
    dir = path_join(s->out_base, s->out_name);
    if (dir_make(dir) == 0 && path_kind(dir) == 1) {
        s->out_dir = dir;
        return 0;
    }
    mem_free(dir);
    /* next to the program was read-only (a CD, Program Files): use the desktop */
    mem_free(s->out_base);
    s->out_base = str_dup(fallback_out_dir());
    dir = path_join(s->out_base, s->out_name);
    if (dir_make(dir) == 0 && path_kind(dir) == 1) {
        s->out_dir = dir;
        return 0;
    }
    mem_free(dir);
    return -1;
}

static void dates(Str *r, const Finding *f)
{
    char a[20], b[11];
    datetime_str(&f->file_date, a);
    date_str(&f->server_date, b);
    if (!a[0] && !b[0]) return;
    str_add(r, "   Dates:    ");
    if (a[0]) str_fmt(r, "%s %s", f->url ? "downloaded" : "file dated", a);
    if (a[0] && b[0]) str_add(r, "; ");
    if (b[0]) str_fmt(r, "the server's copy was dated %s", b);
    str_add(r, "\n");
}

static void finding(Str *r, const Finding *f, int n)
{
    str_fmt(r, "%d. %s\n", n, f->name);
    str_fmt(r, "   What:     %s, %N bytes%s\n", f->what, f->size, f->damaged ? " (incomplete or damaged)" : "");
    if (f->wanted) str_fmt(r, "   WANTED:   this looks like %s\n", f->wanted);
    else if (f->identified) str_fmt(r, "   Looks like: %s\n", f->identified);
    if (f->saved) str_fmt(r, "   Saved as: %s\n", f->saved);
    str_fmt(r, "   Found in: %s\n", f->source);
    if (f->url) str_fmt(r, "   Address:  %s\n", f->url);
    dates(r, f);
    str_fmt(r, "   Why:      %s\n", f->why);
    str_fmt(r, "   SHA-1:    %s\n", f->sha1);
    if (f->copies) str_fmt(r, "   The same file was found in %d more place%s.\n", f->copies, f->copies == 1 ? "" : "s");
    str_add(r, "\n");
}

static void heading(Str *r, const char *title)
{
    size_t i, n = str_len(title);
    str_fmt(r, "\n%s\n", title);
    for (i = 0; i < n; i++) str_addc(r, '-');
    str_add(r, "\n");
}

static void notes(Str *r, const Notes *l)
{
    int i;
    for (i = 0; i < l->n; i++) {
        char d[11];
        date_str(&l->v[i].date, d);
        str_fmt(r, "  %10s  %s", d, l->v[i].text);
        if (l->v[i].count > 1) str_fmt(r, "  (seen %d times)", l->v[i].count);
        str_add(r, "\n");
    }
}

static char *build_report(Scan *s)
{
    Str r = {0};
    Date now;
    char when[20];
    int i, n, new_coke = 0, old_coke = 0, other = 0;
    u32 secs = (ms_now() - s->start_ms) / 1000;
    for (i = 0; i < s->nf; i++) {
        if (s->f[i].known) old_coke++;
        else if (s->f[i].cat == CAT_COKE) new_coke++;
        else other++;
    }
    date_now(&now);
    datetime_str(&now, when);

    str_add(&r, "COKE STUDIOS CACHE FINDER " CCF_VERSION " - RESULTS\n");
    str_add(&r, "=======================================\n\n");
    str_add(&r, "Thank you for searching! Everything listed under \"New\" below is in the\n"
                "zip beside this report. Nothing else from this computer is in it.\n\n");
    str_fmt(&r, "Searched:       %s, on %s\n", when, os_name());
    str_add(&r, "Places:         ");
    for (i = 0; i < s->nplaces; i++) str_fmt(&r, "%s%s", i ? ", " : "", s->places[i]);
    str_fmt(&r, "\nFiles checked:  %N\n", s->files_checked);
    if (s->bytes_read) str_fmt(&r, "Searched inside %M of caches%s\n", s->bytes_read, s->deleted_mode ? " and disk space" : "");
    str_fmt(&r, "Time taken:     %u min %u s\n", secs / 60, secs % 60);
    str_fmt(&r, "Finder:         %s, knowing %d archived files\n", CCF_VERSION, known_count());
    if (s->stopped) str_add(&r, "\nThe search was stopped early, so this is only part of the computer.\n");

    heading(&r, "SUMMARY");
    str_fmt(&r, "  %d new Coke Studios file%s (not in the project's archive yet)\n", new_coke, new_coke == 1 ? "" : "s");
    str_fmt(&r, "  %d Coke Studios file%s the archive already has\n", old_coke, old_coke == 1 ? "" : "s");
    str_fmt(&r, "  %d other Shockwave file%s (could be from the game's first version)\n", other, other == 1 ? "" : "s");
    str_fmt(&r, "  %d old Coke Studios web address%s\n", s->urls.n, s->urls.n == 1 ? "" : "es");
    str_fmt(&r, "  %d other sign%s the game was played here\n", s->notes.n, s->notes.n == 1 ? "" : "s");

    if (new_coke) {
        heading(&r, "NEW COKE STUDIOS FILES (in the zip)");
        for (i = 0, n = 0; i < s->nf; i++)
            if (!s->f[i].known && s->f[i].cat == CAT_COKE) finding(&r, &s->f[i], ++n);
    }
    if (other) {
        heading(&r, "OTHER SHOCKWAVE FILES (in the zip)");
        str_add(&r, "No Coke words were found in these, but the game's first version (2002-2004)\n"
                    "may not have used them, so they are included to be checked by hand.\n\n");
        for (i = 0, n = 0; i < s->nf; i++)
            if (!s->f[i].known && s->f[i].cat == CAT_OTHER) finding(&r, &s->f[i], ++n);
    }
    if (old_coke) {
        heading(&r, "FILES THE ARCHIVE ALREADY HAS (not in the zip)");
        str_add(&r, "Listed because where and when they were saved helps date the archive's copies.\n\n");
        for (i = 0; i < s->nf; i++) {
            const Finding *f = &s->f[i];
            char d[20];
            if (!f->known) continue;
            datetime_str(f->server_date.year ? &f->server_date : &f->file_date, d);
            str_fmt(&r, "  %s = %s%s%s\n      in %s\n", f->name, f->known, d[0] ? ", dated " : "", d, f->source);
        }
    }
    if (s->urls.n) {
        heading(&r, "OLD COKE STUDIOS ADDRESSES");
        str_add(&r, "From browser history and caches. Query strings were removed: they can hold\n"
                    "session ids and screen names.\n\n");
        notes(&r, &s->urls);
    }
    if (s->notes.n) {
        heading(&r, "OTHER SIGNS");
        notes(&r, &s->notes);
    }
    if (s->named.n) {
        heading(&r, "FILES WITH COKE IN THEIR NAME (not in the zip)");
        str_add(&r, "These might be screenshots, saved pages or downloads from the game. They were\n"
                    "not included because they could be personal. Have a look, and if any are from\n"
                    "Coke Studios, please send them too.\n\n");
        notes(&r, &s->named);
    }
    str_add(&r, "\nWhere to send this: " CCF_PROJECT_URL "\n");
    return r.s;
}

static int add_to_zip(ZipWriter *z, const char *out_name, const char *rel, const char *full, const Date *d)
{
    Buf b = {0};
    char *entry = fmt("%s/%s", out_name, rel);
    int r = read_file(full, MAX_FILE, &b, NULL);
    if (!r) r = zip_add(z, entry, b.p ? b.p : (const u8 *)"", b.len, d);
    buf_free(&b);
    mem_free(entry);
    return r;
}

int write_results(Scan *s, int make_zip, char **zip_path)
{
    char *report, *path;
    PFile *f;
    int i, failed = 0;
    *zip_path = NULL;
    if (!s->nf && !s->urls.n && !s->notes.n && !s->named.n) return 1;
    if (ensure_out_dir(s)) return -1;
    report = build_report(s);
    path = path_join(s->out_dir, "report.txt");
    f = f_open_write(path);
    if (!f || f_write(f, report, (u32)str_len(report))) failed = 1;
    if (f) f_close(f);
    mem_free(report);
    if (failed) {
        mem_free(path);
        return -1;
    }
    if (make_zip) {
        char *zp = fmt("%s.zip", s->out_dir);
        ZipWriter *z = zip_create(zp);
        if (z) {
            int bad = add_to_zip(z, s->out_name, "report.txt", path, NULL);
            for (i = 0; i < s->nf && !bad; i++) {
                char *full;
                if (!s->f[i].saved) continue;
                full = path_join(s->out_dir, s->f[i].saved + 0);
                bad = add_to_zip(z, s->out_name, s->f[i].saved, full,
                                 s->f[i].server_date.year ? &s->f[i].server_date : &s->f[i].file_date);
                mem_free(full);
            }
            if (zip_close(z) == 0 && !bad) *zip_path = zp;
            else mem_free(zp);
        } else {
            mem_free(zp);
        }
    }
    mem_free(path);
    return 0;
}

/* ---- --make-known ---- */

typedef struct {
    char hex[41];
    u32 size;
    char *label;
} Known;

typedef struct {
    Known *v;
    int n, cap;
} KnownList;

static void fingerprint(KnownList *l, const char *path, const char *label, u64 size)
{
    Buf b = {0};
    Sniff sn;
    u32 len;
    u8 digest[20];
    int i;
    const char *ext = path_ext(path);
    if (size == 0 || size > MAX_FILE || read_file(path, 64, &b, NULL)) {
        buf_free(&b);
        return;
    }
    if (!(b.len >= 8 && sniff(b.p, b.len, &sn, 0)) &&
        !((str_ieq(ext, "txt") || str_ieq(ext, "xml")) && size < (8u << 20))) {
        buf_free(&b);
        return;
    }
    if (read_file(path, MAX_FILE, &b, NULL)) {
        buf_free(&b);
        return;
    }
    len = b.len;
    if (b.len >= 8 && sniff(b.p, b.len, &sn, 0)) {
        Exam x;
        examine(b.p, b.len, &sn, 0, &x);
        len = x.length;
        exam_free(&x);
    }
    sha1(b.p, len, digest);
    buf_free(&b);
    if (l->n == l->cap) {
        int cap = l->cap ? l->cap * 2 : 256;
        Known *v = (Known *)mem_realloc(l->v, (size_t)cap * sizeof *v);
        if (!v) return;
        l->v = v;
        l->cap = cap;
    }
    sha1_hex(digest, l->v[l->n].hex);
    for (i = 0; i < l->n; i++)
        if (str_eq(l->v[i].hex, l->v[l->n].hex)) return; /* the first name wins */
    l->v[l->n].size = len;
    l->v[l->n].label = str_dup(label);
    l->n++;
}

/* Only the file name is recorded: where it sits on the maintainer's disk is
 * nobody else's business, and the report never needs it. */
static void fingerprint_tree(KnownList *l, const char *dir)
{
    int n, i;
    DirEntry *e = dir_list(dir, &n);
    if (!e) return;
    for (i = 0; i < n; i++) {
        char *p = path_join(dir, e[i].name);
        if (e[i].is_dir) fingerprint_tree(l, p);
        else fingerprint(l, p, e[i].name, e[i].size);
        mem_free(p);
    }
    dir_list_free(e, n);
}

int make_known(const char *out_path, char **dirs, int ndirs)
{
    KnownList l = {0};
    Str c = {0};
    PFile *f;
    int i, j, gap;
    for (i = 0; i < ndirs; i++) {
        char *d = str_dup(dirs[i]);
        size_t n = str_len(d);
        while (n > 1 && (d[n - 1] == '\\' || d[n - 1] == '/')) d[--n] = 0;
        say("Fingerprinting %s ...\n", d);
        fingerprint_tree(&l, d);
        mem_free(d);
    }
    for (gap = l.n / 2; gap > 0; gap /= 2) /* shell sort by hash */
        for (i = gap; i < l.n; i++) {
            Known t = l.v[i];
            for (j = i; j >= gap && memcmp(l.v[j - gap].hex, t.hex, 40) > 0; j -= gap) l.v[j] = l.v[j - gap];
            l.v[j] = t;
        }
    str_add(&c, "/* Generated by CokeCacheFinder --make-known: every Shockwave, Flash, text and\n"
                " * XML file in the project's archive, by SHA-1. Do not edit by hand. */\n"
                "#include \"known.h\"\n\n");
    str_fmt(&c, "const KnownFile KNOWN_FILES[%d] = {\n", l.n ? l.n : 1);
    for (i = 0; i < l.n; i++) {
        const char *p;
        str_fmt(&c, "    {\"%s\", %uu, \"", l.v[i].hex, l.v[i].size);
        for (p = l.v[i].label; *p; p++) {
            if (*p == '"' || *p == '\\' || *p == '?') str_addc(&c, '\\');
            str_addc(&c, *p);
        }
        str_add(&c, "\"},\n");
    }
    if (!l.n) str_add(&c, "    {\"\", 0, \"\"},\n");
    str_fmt(&c, "};\nconst int KNOWN_FILES_COUNT = %d;\n", l.n);
    f = f_open_write(out_path);
    if (!f || f_write(f, c.s, (u32)c.len)) {
        say("Could not write %s\n", out_path);
        if (f) f_close(f);
        return 1;
    }
    f_close(f);
    say("Wrote %d fingerprints to %s\n", l.n, out_path);
    return 0;
}
