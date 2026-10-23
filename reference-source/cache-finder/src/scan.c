/* Walking drives and folders, and searching inside files.
 *
 * Every file gets its first bytes read, which is enough to recognise a
 * Shockwave or Flash file however it was renamed. Files in browser caches
 * and temp folders are also searched end to end ("carved"), because some
 * browsers pack many downloads into one file. A raw disk is carved the same
 * way, which is how files deleted years ago can still turn up. */
#include "finder.h"

enum { MODE_NORMAL, MODE_WINDOWS, MODE_SYSTEM };

static const char *const SKIP_ALWAYS[] = {
    "System Volume Information", "$WinREAgent", "$Windows.~BT", "$Windows.~WS", "$SysReset",
    "$GetCurrent", "Config.Msi", "WinSxS", "WindowsApps", "node_modules", ".git", ".svn", ".hg",
    ".Spotlight-V100", ".fseventsd", ".DocumentRevisions-V100", ".Trashes.noindex",
    "Cookies", "INetCookies", /* personal, and never a file of the game's */
    0};

/* Inside C:\Windows only these can hold anything of a user's: Windows
 * 95/98/ME kept the browser cache, profiles and desktop there. */
static const char *const WINDOWS_KEEP[] = {
    "Temporary Internet Files", "Profiles", "Application Data", "Local Settings", "Temp",
    "Desktop", "History", "Favorites", "Offline Web Pages", "Downloaded Program Files", 0};
static const char *const SYSTEM_DIRS[] = {"System32", "System", "SysWOW64", 0};
/* Shockwave lived in System32\Macromed; its Prefs folder is there. */
static const char *const SYSTEM_KEEP[] = {"Macromed", "Adobe", 0};

/* Never worth walking from a Mac or Linux root. */
static const char *const ROOT_SKIP[] = {
    "proc", "sys", "dev", "run", "snap", "usr", "bin", "sbin", "lib", "lib32", "lib64",
    "libx32", "boot", "etc", "System", "cores", "net", "Network", "Volumes", 0};

/* Outside caches, files of these types are not opened: none of them can be
 * a Shockwave or Flash file, and a modern PC holds hundreds of thousands of
 * them (each open is also an antivirus scan). Caches are always read, since
 * browsers name their files however they like. */
static const char *const NEVER_GAME[] = {
    "dll", "sys", "exe", "msi", "msp", "cab", "mui", "cat", "pdb", "obj", "lib", "o", "a", "so", "dylib",
    "png", "jpg", "jpeg", "gif", "bmp", "ico", "svg", "webp", "tif", "tiff", "psd", "heic",
    "mp3", "mp4", "m4a", "m4v", "wav", "ogg", "flac", "wma", "wmv", "avi", "mkv", "mov", "webm", "mid",
    "pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "odt", "rtf", "epub",
    "js", "mjs", "cjs", "ts", "tsx", "jsx", "json", "map", "css", "scss", "less", "html", "htm", "xml",
    "txt", "md", "log", "ini", "cfg", "yml", "yaml", "toml", "csv", "lnk", "pf", "etl", "evtx",
    "py", "pyc", "pyd", "java", "class", "jar", "c", "h", "cpp", "hpp", "cs", "go", "rs", "rb", "php",
    "sh", "bat", "cmd", "ps1", "lua", "wasm", "node", "ttf", "otf", "woff", "woff2", "eot",
    "zip", "rar", "7z", "gz", "tgz", "xz", "bz2", "zst", "vhd", "vhdx", "vmdk", "qcow2", "vdi", "dmg",
    "iso", "img", "bin", "dat", "db", "sqlite", "idx", "pak", 0}; /* images are searched when named */

static int in_list(const char *name, const char *const *list)
{
    int i;
    for (i = 0; list[i]; i++)
        if (str_ieq(name, list[i])) return 1;
    return 0;
}

#define WINDOW (4u << 20)
#define TAIL 1024u

/* Something that looked like a Shockwave or Flash file began at `at`: read
 * it out, check it really is one and hand it on. Returns its length. */
static u32 take(Scan *s, const char *path, const char *label, PFile *f, u64 size, u64 at,
                const u8 *head, u32 avail, const Date *d)
{
    Sniff sn;
    Exam x;
    Buf b = {0};
    Blob bl;
    u32 want, got = 0, len;
    char *name;
    if (!sniff(head, avail, &sn, 1)) return 0;
    want = sn.cws ? (sn.length + 4096 > MAX_FILE ? MAX_FILE : (u32)sn.length + 4096) : (u32)sn.length;
    if (size && at + want > size) want = (u32)(size - at);
    if (buf_reserve(&b, want) || f_read_at(f, at, b.p, want, &got) || got < 16) {
        buf_free(&b);
        return 0;
    }
    b.len = got;
    if (!examine(b.p, got, &sn, 1, &x)) {
        exam_free(&x);
        buf_free(&b);
        return 0;
    }
    len = x.length;
    exam_free(&x);
    name = fmt("%s_at_%U", label, at);
    memset(&bl, 0, sizeof bl);
    bl.data = b.p;
    bl.len = len;
    bl.path = path;
    bl.offset = at;
    bl.carved = 1;
    bl.name = name;
    if (d) bl.file_date = *d;
    consider(s, &bl);
    mem_free(name);
    buf_free(&b);
    return len;
}

void carve(Scan *s, const char *path, PFile *f, u64 size, const Date *d, int raw)
{
    u8 *buf = (u8 *)mem_alloc(WINDOW);
    u64 off = 0, skip_until = 0;
    char *label;
    const char *show = raw ? path : path_base(path);
    if (!buf) return;
    label = str_dup(raw ? "disk" : path_base(path));
    for (;;) {
        u32 got = 0, end, i;
        int last;
        if (stop_requested()) {
            s->stopped = 1;
            break;
        }
        if (f_read_at(f, off, buf, WINDOW, &got) || got == 0) break;
        last = got < WINDOW || (size && off + got >= size);
        end = last ? got : got - TAIL;
        for (i = 0; i < end; i++) {
            u8 c = buf[i], lc;
            if (off + i < skip_until) {
                u64 jump = skip_until - off;
                if (jump >= end) break;
                i = (u32)jump;
                c = buf[i];
            }
            if ((c == 'X' && i + 16 <= got && !memcmp(buf + i, "XFIR", 4)) ||
                (c == 'R' && i + 16 <= got && !memcmp(buf + i, "RIFX", 4)) ||
                ((c == 'F' || c == 'C') && i + 9 <= got && buf[i + 1] == 'W' && buf[i + 2] == 'S')) {
                u32 used = take(s, path, label, f, size, off + i, buf + i, got - i, d);
                if (used) {
                    skip_until = off + i + used;
                    continue;
                }
            }
            lc = (u8)(c | 32);
            if (lc == 'c' || lc == 'm' || lc == 's' || lc == 'a' || lc == 'f') {
                int k = url_at(s, buf, i, got, d);
                if (k > 0) i += (u32)k - 1;
            }
        }
        s->bytes_read += end;
        off += end;
        if (raw || size > (64u << 20)) {
            char *msg = size ? fmt("%s: %U of %U MB", show, off >> 20, size >> 20)
                             : fmt("%s: %U MB", show, off >> 20);
            status(s, "reading", msg);
            mem_free(msg);
        }
        if (last) break;
    }
    mem_free(label);
    mem_free(buf);
}

static void whole_file(Scan *s, const char *path, PFile *f, const DirEntry *e, int text_hint)
{
    Buf b = {0};
    Blob bl;
    u32 want = e->size > MAX_FILE ? MAX_FILE : (u32)e->size, got = 0;
    if (buf_reserve(&b, want) || f_read_at(f, 0, b.p, want, &got)) {
        buf_free(&b);
        return;
    }
    memset(&bl, 0, sizeof bl);
    bl.data = b.p;
    bl.len = got;
    bl.path = path;
    bl.name = e->name;
    bl.file_date = e->mtime;
    bl.text_hint = text_hint;
    consider(s, &bl);
    buf_free(&b);
}

static void check_file(Scan *s, const char *dir, const DirEntry *e, int in_cache)
{
    const char *name = e->name, *ext = path_ext(name);
    char *path;
    PFile *f;
    u8 head[64];
    u32 got = 0;
    Sniff sn;
    int carve_it, special;

    if (e->size == 0) return;
    path = path_join(dir, name);
    if (is_done(s, path)) {
        mem_free(path);
        return;
    }
    s->files_checked++;
    status(s, "searching", path);

    /* setPref("sfsn", screenName): Shockwave remembered who played here */
    if (str_ieq(name, "sfsn") || str_ieq(name, "sfsn.txt")) {
        char *r = redact(path), *t = fmt("Shockwave remembered a Coke Studios screen name in %s (the name itself was not read)", r);
        add_note(s, t, &e->mtime);
        mem_free(r);
        mem_free(t);
        mem_free(path);
        return;
    }
    if (str_ieq(ext, "url") && e->size < 65536) { /* Internet Explorer favourites */
        Buf b = {0};
        if (!read_file(path, 65536, &b, NULL)) urls_in(s, b.p, b.len, &e->mtime);
        buf_free(&b);
        mem_free(path);
        return;
    }
    /* swap files hold whatever was in memory: Windows 9x's, NT's, and Mac
     * Internet Explorer's single-file cache. A screensaver is a program
     * with its Shockwave or Flash movie packed inside. */
    special = str_ieq(name, "pagefile.sys") || str_ieq(name, "win386.swp") || str_ieq(name, "swapfile.sys") ||
              str_ifind(name, "ms internet cache") || (str_ieq(ext, "scr") && e->size < (64u << 20)) ||
              ((name_is_cokeish(name) || str_ifind(name, "modtool") || str_ifind(name, "mod tool")) &&
               e->size < (64u << 20));
    if (!special && !in_cache && in_list(ext, NEVER_GAME) && !text_name_wanted(name)) {
        int cokeish = name_is_cokeish(name) || str_ifind(name, "modtool") || str_ifind(name, "mod tool");
        /* a projector (the Mod Tool), or someone's backup: casts and
         * compressed Flash are stored in archives as they are, so they can
         * be found inside one without unpacking it */
        int packed = str_ieq(ext, "exe") || str_ieq(ext, "zip") || str_ieq(ext, "rar") || str_ieq(ext, "7z");
        if (name_is_cokeish(name)) add_named(s, path, e->size, &e->mtime);
        if (cokeish && packed && e->size < (512u << 20)) {
            PFile *pf = f_open_read(path);
            if (pf) {
                carve(s, path, pf, e->size, &e->mtime, 0);
                f_close(pf);
            }
        }
        mem_free(path);
        return;
    }

    f = f_open_read(path);
    if (!f) {
        mem_free(path);
        return;
    }
    if (!f_read_at(f, 0, head, sizeof head, &got) && got >= 8 && sniff(head, got, &sn, 0)) {
        whole_file(s, path, f, e, 0);
        f_close(f);
        mem_free(path);
        return;
    }
    if ((text_name_wanted(name) || (in_cache && (str_ieq(ext, "txt") || str_ieq(ext, "xml")))) &&
        e->size < (4u << 20))
        whole_file(s, path, f, e, 1);
    if (name_is_cokeish(name)) add_named(s, path, e->size, &e->mtime);

    /* A cache file last written after 2015 cannot hold anything from a game
     * that closed in 2007; skipping them keeps a modern PC's search short. */
    carve_it = in_cache && (e->mtime.year == 0 || e->mtime.year < 2016);
    if (carve_it || special) carve(s, path, f, e->size, &e->mtime, 0);
    f_close(f);
    mem_free(path);
}

static void walk(Scan *s, const char *dir, int depth, int mode, int in_cache)
{
    DirEntry *e;
    int n, i, root = str_eq(dir, "/");
    if (stop_requested()) {
        s->stopped = 1;
        return;
    }
    if (depth > 48) return;
    if (s->out_dir && str_ieq(dir, s->out_dir)) return;
    e = dir_list(dir, &n);
    if (!e) return;
    s->dirs_checked++;

    for (i = 0; i < n; i++) {
        if (e[i].is_dir) continue;
        if (str_ieq(e[i].name, "index.dat")) ie_index(s, dir);
        else if (str_ieq(e[i].name, "_CACHE_MAP_")) ff_cache(s, dir);
    }
    /* in C:\Windows and System32 only screensavers: MyCoke offered them as
     * downloads, and Windows installs them there */
    for (i = 0; i < n && !s->stopped; i++)
        if (!e[i].is_dir && (mode == MODE_NORMAL || str_ieq(path_ext(e[i].name), "scr")))
            check_file(s, dir, &e[i], in_cache);

    for (i = 0; i < n && !s->stopped; i++) {
        const char *name = e[i].name;
        int child = MODE_NORMAL;
        char *sub;
        if (!e[i].is_dir || in_list(name, SKIP_ALWAYS)) continue;
        if (str_istarts(name, "CokeStudiosFinds-")) continue; /* an earlier run's results */
        if (root && in_list(name, ROOT_SKIP)) continue;
        if (mode == MODE_WINDOWS) {
            if (in_list(name, SYSTEM_DIRS)) child = MODE_SYSTEM;
            else if (!in_list(name, WINDOWS_KEEP)) continue;
        } else if (mode == MODE_SYSTEM) {
            if (!in_list(name, SYSTEM_KEEP)) continue;
        } else if (depth <= 2 && (str_ieq(name, "Windows") || str_ieq(name, "WINNT"))) {
            child = MODE_WINDOWS;
        }
        /* Windows Store apps' private data: large, and all post-2012 */
        if (str_ieq(name, "Packages") && str_ieq(path_base(dir), "Local")) continue;
        sub = path_join(dir, name);
        walk(s, sub, depth + 1, child, in_cache || cachey_dir(name));
        mem_free(sub);
    }
    dir_list_free(e, n);
}

void scan_tree(Scan *s, const char *root)
{
    walk(s, root, 0, MODE_NORMAL, cachey_dir(path_base(root)));
}

int scan_raw(Scan *s, const char *device)
{
    PFile *f = f_open_read(device);
    if (!f) return f_last_error_denied() ? -2 : -1;
    carve(s, device, f, f_size(f), NULL, 1);
    f_close(f);
    return 0;
}

/* Something the user pointed at: a folder, a drive, a disk image, a device. */
void scan_path(Scan *s, const char *path)
{
    int kind;
    if (path_is_raw(path)) {
        if (scan_raw(s, path) == -2) say("  Could not read %s - reading a raw disk needs administrator rights.\n", path);
        return;
    }
    kind = path_kind(path);
    if (kind == 1) {
        scan_tree(s, path);
    } else if (kind == 0) {
        PFile *f = f_open_read(path);
        DirEntry e;
        u8 head[64];
        u32 got = 0;
        Sniff sn;
        if (!f) {
            say("  Could not open %s\n", path);
            return;
        }
        memset(&e, 0, sizeof e);
        e.name = (char *)path_base(path);
        e.size = f_size(f);
        s->files_checked++;
        if (!f_read_at(f, 0, head, sizeof head, &got) && got >= 8 && sniff(head, got, &sn, 0))
            whole_file(s, path, f, &e, 0);
        else
            carve(s, path, f, e.size, NULL, 0); /* a disk image, or any container */
        f_close(f);
    } else {
        say("  %s does not exist.\n", path);
    }
}
