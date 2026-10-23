/* macOS and Linux. Also how an old Windows drive gets searched from a Mac
 * or a Linux live USB: mount it and point the finder at it. */
#ifndef _WIN32
#define _FILE_OFFSET_BITS 64
#define _DEFAULT_SOURCE
#define _DARWIN_C_SOURCE
#include <sys/types.h>
#include <sys/stat.h>
#include <sys/time.h>
#include <sys/utsname.h>
#include <sys/ioctl.h>
#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <signal.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#ifdef __APPLE__
#include <mach-o/dyld.h>
#include <sys/disk.h>
#endif
#ifdef __linux__
#include <linux/fs.h>
#endif
#include "finder.h"

void *mem_alloc(size_t n) { return calloc(1, n ? n : 1); }
void *mem_realloc(void *p, size_t n) { return realloc(p, n ? n : 1); }
void mem_free(void *p) { free(p); }

/* ---- console ---- */

static volatile sig_atomic_t g_stop;

static void on_int(int sig)
{
    (void)sig;
    g_stop = 1;
}

int stop_requested(void) { return g_stop != 0; }
void con_clear_stop(void) { g_stop = 0; }
int con_owned(void) { return 0; }
int con_interactive(void) { return isatty(1); }

void con_write(const char *text)
{
    size_t n = strlen(text);
    while (n) {
        ssize_t w = write(1, text, n);
        if (w <= 0) {
            if (w < 0 && errno == EINTR) continue;
            return;
        }
        text += w;
        n -= (size_t)w;
    }
}

int con_read_line(char *buf, int cap)
{
    int n = 0;
    char c;
    for (;;) {
        ssize_t r = read(0, &c, 1);
        if (r < 0 && errno == EINTR) continue;
        if (r <= 0) {
            if (!n) return -1;
            break;
        }
        if (c == '\n') break;
        if (n < cap - 1) buf[n++] = c;
    }
    while (n > 0 && buf[n - 1] == '\r') n--;
    buf[n] = 0;
    return n;
}

/* ---- files ---- */

struct PFile {
    int fd;
    int raw;
    u8 *bounce;
    u32 bounce_cap;
};

static int g_open_errno;

int path_is_raw(const char *p) { return strncmp(p, "/dev/", 5) == 0; }

static PFile *wrap(int fd, int raw)
{
    PFile *f;
    if (fd < 0) {
        g_open_errno = errno;
        return NULL;
    }
    f = (PFile *)mem_alloc(sizeof *f);
    if (!f) {
        close(fd);
        return NULL;
    }
    f->fd = fd;
    f->raw = raw;
    return f;
}

PFile *f_open_read(const char *path) { return wrap(open(path, O_RDONLY), path_is_raw(path)); }
PFile *f_open_write(const char *path) { return wrap(open(path, O_WRONLY | O_CREAT | O_TRUNC, 0644), 0); }
int f_last_error_denied(void) { return g_open_errno == EACCES || g_open_errno == EPERM; }

static int pread_all(int fd, u64 off, u8 *buf, u32 n, u32 *got)
{
    *got = 0;
    while (*got < n) {
        ssize_t r = pread(fd, buf + *got, n - *got, (off_t)(off + *got));
        if (r < 0 && errno == EINTR) continue;
        if (r < 0) return *got ? 0 : -1;
        if (r == 0) break;
        *got += (u32)r;
    }
    return 0;
}

int f_read_at(PFile *f, u64 off, void *buf, u32 n, u32 *got)
{
    u64 base;
    u32 skip, total, have = 0;
    if (!f->raw) return pread_all(f->fd, off, (u8 *)buf, n, got);
    /* character devices (macOS /dev/rdiskN) take whole sectors only */
    base = off & ~(u64)4095;
    skip = (u32)(off - base);
    total = (skip + n + 4095) & ~4095u;
    *got = 0;
    if (total > f->bounce_cap) {
        free(f->bounce);
        f->bounce = (u8 *)malloc(total);
        f->bounce_cap = f->bounce ? total : 0;
        if (!f->bounce) return -1;
    }
    if (pread_all(f->fd, base, f->bounce, total, &have)) return -1;
    if (have <= skip) return 0;
    *got = have - skip < n ? have - skip : n;
    memcpy(buf, f->bounce + skip, *got);
    return 0;
}

int f_write(PFile *f, const void *buf, u32 n)
{
    const u8 *p = (const u8 *)buf;
    while (n) {
        ssize_t w = write(f->fd, p, n);
        if (w < 0 && errno == EINTR) continue;
        if (w <= 0) return -1;
        p += w;
        n -= (u32)w;
    }
    return 0;
}

u64 f_size(PFile *f)
{
    struct stat st;
    if (fstat(f->fd, &st) == 0 && S_ISREG(st.st_mode)) return (u64)st.st_size;
#if defined(__linux__) && defined(BLKGETSIZE64)
    {
        unsigned long long sz = 0;
        if (ioctl(f->fd, BLKGETSIZE64, &sz) == 0) return sz;
    }
#endif
#ifdef __APPLE__
    {
        uint64_t count = 0;
        uint32_t bs = 0;
        if (ioctl(f->fd, DKIOCGETBLOCKCOUNT, &count) == 0 && ioctl(f->fd, DKIOCGETBLOCKSIZE, &bs) == 0)
            return count * bs;
    }
#endif
    {
        off_t end = lseek(f->fd, 0, SEEK_END);
        return end > 0 ? (u64)end : 0;
    }
}

void f_close(PFile *f)
{
    if (!f) return;
    close(f->fd);
    free(f->bounce);
    free(f);
}

int dir_make(const char *path) { return mkdir(path, 0755) == 0 || errno == EEXIST ? 0 : -1; }

int path_kind(const char *path)
{
    struct stat st;
    if (stat(path, &st)) return -1;
    return S_ISDIR(st.st_mode) ? 1 : 0;
}

static void tm_date(const struct tm *t, Date *d)
{
    d->year = (u16)(t->tm_year + 1900);
    d->month = (u8)(t->tm_mon + 1);
    d->day = (u8)t->tm_mday;
    d->hour = (u8)t->tm_hour;
    d->minute = (u8)t->tm_min;
    d->second = (u8)t->tm_sec;
}

DirEntry *dir_list(const char *path, int *count)
{
    DIR *d = opendir(path);
    DirEntry *v = NULL;
    int n = 0, cap = 0;
    struct dirent *de;
    *count = 0;
    if (!d) return NULL;
    while ((de = readdir(d))) {
        struct stat st;
        struct tm tmv;
        char *full;
        time_t mt;
        if (!strcmp(de->d_name, ".") || !strcmp(de->d_name, "..")) continue;
        full = path_join(path, de->d_name);
        if (lstat(full, &st) || !(S_ISREG(st.st_mode) || S_ISDIR(st.st_mode))) { /* no links, devices, pipes */
            free(full);
            continue;
        }
        free(full);
        if (n == cap) {
            DirEntry *nv;
            cap = cap ? cap * 2 : 64;
            nv = (DirEntry *)realloc(v, (size_t)cap * sizeof *nv);
            if (!nv) break;
            v = nv;
        }
        memset(&v[n], 0, sizeof v[n]);
        v[n].name = str_dup(de->d_name);
        v[n].is_dir = S_ISDIR(st.st_mode);
        v[n].size = (u64)st.st_size;
        mt = st.st_mtime;
        if (gmtime_r(&mt, &tmv)) tm_date(&tmv, &v[n].mtime);
        n++;
    }
    closedir(d);
    *count = n;
    if (!v) v = (DirEntry *)mem_alloc(sizeof *v);
    return v;
}

void dir_list_free(DirEntry *e, int count)
{
    int i;
    for (i = 0; i < count; i++) free(e[i].name);
    free(e);
}

char path_sep(void) { return '/'; }

void filetime_to_date(u64 ft, Date *d)
{
    u64 secs = ft / 10000000u;
    memset(d, 0, sizeof *d);
    if (secs < 11644473600ull || secs - 11644473600ull > 0xFFFFFFFFull) return;
    date_from_unix((u32)(secs - 11644473600ull), d);
}

void date_now(Date *d)
{
    time_t t = time(NULL);
    struct tm tmv;
    memset(d, 0, sizeof *d);
    if (localtime_r(&t, &tmv)) tm_date(&tmv, d);
}

u32 ms_now(void)
{
    struct timeval tv;
    gettimeofday(&tv, NULL);
    return (u32)((u64)tv.tv_sec * 1000u + (u64)tv.tv_usec / 1000u);
}

u32 random_seed(void)
{
    struct timeval tv;
    u32 x;
    gettimeofday(&tv, NULL);
    x = (u32)tv.tv_sec ^ ((u32)tv.tv_usec << 12) ^ ((u32)getpid() << 20);
    x ^= x >> 13;
    x *= 0x5bd1e995u;
    return x ^ (x >> 15);
}

/* ---- places ---- */

char **search_roots(int *count)
{
    static char *roots[64];
    int n = 0;
    roots[n++] = (char *)"/";
#ifdef __APPLE__
    {
        /* other disks, including an old Windows drive on a USB adapter */
        struct stat root, st;
        DIR *d = opendir("/Volumes");
        struct dirent *de;
        stat("/", &root);
        while (d && (de = readdir(d)) && n < 64) {
            char *p;
            if (de->d_name[0] == '.') continue;
            p = path_join("/Volumes", de->d_name);
            if (lstat(p, &st) == 0 && S_ISDIR(st.st_mode) && st.st_dev != root.st_dev) roots[n++] = p;
            else free(p);
        }
        if (d) closedir(d);
    }
#endif
    *count = n;
    return roots;
}

char **raw_volumes(int *count)
{
    static char *none[1];
    *count = 0; /* name the device instead: sudo ./CokeCacheFinder /dev/sdb */
    return none;
}

int raw_supported(void) { return 0; }

const char *exe_dir(void)
{
    static char dir[4096];
    char buf[4096];
    char *slash;
    if (dir[0]) return dir;
    buf[0] = 0;
#ifdef __APPLE__
    {
        uint32_t size = sizeof buf;
        char real[PATH_MAX];
        if (_NSGetExecutablePath(buf, &size) == 0 && realpath(buf, real)) strncpy(buf, real, sizeof buf - 1);
    }
#else
    {
        ssize_t n = readlink("/proc/self/exe", buf, sizeof buf - 1);
        buf[n > 0 ? n : 0] = 0;
    }
#endif
    buf[sizeof buf - 1] = 0;
    slash = strrchr(buf, '/');
    if (!buf[0] || !slash) return strcpy(dir, ".");
    if (slash == buf) slash++;
    *slash = 0;
    memcpy(dir, buf, strlen(buf) + 1);
    return dir;
}

const char *fallback_out_dir(void)
{
    static char *dir;
    const char *home = getenv("HOME");
    char *desk;
    if (dir) return dir;
    if (!home || !*home) return dir = str_dup("/tmp");
    desk = path_join(home, "Desktop");
    if (path_kind(desk) == 1) return dir = desk;
    free(desk);
    return dir = str_dup(home);
}

const char *os_name(void)
{
    static char name[160];
    struct utsname u;
    if (name[0]) return name;
    if (uname(&u)) return "Unix";
#ifdef __APPLE__
    {
        char *t = fmt("macOS (Darwin %s, %s)", u.release, u.machine);
        strncpy(name, t, sizeof name - 1);
        free(t);
    }
#else
    {
        char *t = fmt("%s %s (%s)", u.sysname, u.release, u.machine);
        strncpy(name, t, sizeof name - 1);
        free(t);
    }
#endif
    return name;
}

int open_folder(const char *path)
{
    pid_t pid = fork();
    if (pid == 0) {
        int null = open("/dev/null", O_WRONLY);
        if (null >= 0) {
            dup2(null, 1);
            dup2(null, 2);
        }
#ifdef __APPLE__
        execlp("open", "open", path, (char *)NULL);
#else
        execlp("xdg-open", "xdg-open", path, (char *)NULL);
#endif
        _exit(127);
    }
    return pid > 0 ? 0 : -1;
}

int relaunch_elevated(const char *args)
{
    (void)args;
    return 0;
}

int main(int argc, char **argv)
{
    struct sigaction sa;
    memset(&sa, 0, sizeof sa);
    sa.sa_handler = on_int;
    sa.sa_flags = SA_RESETHAND; /* a second Ctrl+C really quits */
    sigaction(SIGINT, &sa, NULL);
    signal(SIGPIPE, SIG_IGN);
    return app_main(argc, argv);
}

#endif /* !_WIN32 */
