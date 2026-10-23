/* Windows, from 95 to 11, in one .exe.
 *
 * Three rules make that work. No C runtime is linked: a modern compiler's
 * runtime calls functions Windows 95 and XP do not have, so the few things
 * the C library would do are done here or in util.c. Only functions that
 * exist in Windows 95's kernel32 are imported directly; everything newer
 * (all the Unicode "W" calls, ShellExecute, ...) is looked up while running
 * and used only when it is there. And on 95/98/ME paths stay in the
 * system's ANSI code page, while NT-family Windows gets UTF-16 with the
 * \\?\ prefix, so long and non-English paths work too.
 *
 * build.bat compiles this without SSE (Pentium II-era CPUs lack it) and
 * stamps the .exe as needing Windows 4.0. */
#ifdef _WIN32
#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <intrin.h>
#include "util.h"

/* ---- what the C runtime would have provided ---- */

#pragma function(memset, memcpy, memcmp)
void *memset(void *d, int c, size_t n)
{
    __stosb((unsigned char *)d, (unsigned char)c, n);
    return d;
}

void *memcpy(void *d, const void *s, size_t n)
{
    __movsb((unsigned char *)d, (const unsigned char *)s, n);
    return d;
}

void *memmove(void *d, const void *s, size_t n)
{
    volatile unsigned char *dp = (unsigned char *)d;
    const unsigned char *sp = (const unsigned char *)s;
    if (dp <= sp || dp >= sp + n) {
        __movsb((unsigned char *)d, sp, n);
    } else {
        while (n) {
            n--;
            dp[n] = sp[n];
        }
    }
    return d;
}

int memcmp(const void *a, const void *b, size_t n)
{
    const volatile unsigned char *x = (const unsigned char *)a;
    const unsigned char *y = (const unsigned char *)b;
    for (; n; n--, x++, y++)
        if (*x != *y) return *x - *y;
    return 0;
}

int _fltused = 0;

/* 64-bit shifts on 32-bit x86 are calls to these runtime helpers: value in
 * EDX:EAX, count in CL, result in EDX:EAX. */
#if defined(_M_IX86)
__declspec(naked) void _aullshr(void)
{
    __asm {
        cmp cl, 64
        jae clear
        cmp cl, 32
        jae upper
        shrd eax, edx, cl
        shr edx, cl
        ret
    upper:
        mov eax, edx
        xor edx, edx
        and cl, 31
        shr eax, cl
        ret
    clear:
        xor eax, eax
        xor edx, edx
        ret
    }
}

__declspec(naked) void _allshl(void)
{
    __asm {
        cmp cl, 64
        jae clear
        cmp cl, 32
        jae upper
        shld edx, eax, cl
        shl eax, cl
        ret
    upper:
        mov edx, eax
        xor eax, eax
        and cl, 31
        shl edx, cl
        ret
    clear:
        xor eax, eax
        xor edx, edx
        ret
    }
}
#endif

/* ---- functions newer than Windows 95, found at run time ---- */

typedef HANDLE(WINAPI *CreateFileW_t)(LPCWSTR, DWORD, DWORD, LPSECURITY_ATTRIBUTES, DWORD, DWORD, HANDLE);
typedef HANDLE(WINAPI *FindFirstFileW_t)(LPCWSTR, LPWIN32_FIND_DATAW);
typedef BOOL(WINAPI *FindNextFileW_t)(HANDLE, LPWIN32_FIND_DATAW);
typedef BOOL(WINAPI *CreateDirectoryW_t)(LPCWSTR, LPSECURITY_ATTRIBUTES);
typedef DWORD(WINAPI *GetFileAttributesW_t)(LPCWSTR);
typedef DWORD(WINAPI *GetModuleFileNameW_t)(HMODULE, LPWSTR, DWORD);
typedef LPWSTR(WINAPI *GetCommandLineW_t)(void);
typedef BOOL(WINAPI *WriteConsoleW_t)(HANDLE, const VOID *, DWORD, LPDWORD, LPVOID);
typedef BOOL(WINAPI *ReadConsoleW_t)(HANDLE, LPVOID, DWORD, LPDWORD, LPVOID);
typedef DWORD(WINAPI *GetEnvironmentVariableW_t)(LPCWSTR, LPWSTR, DWORD);
typedef BOOL(WINAPI *CreateProcessW_t)(LPCWSTR, LPWSTR, LPSECURITY_ATTRIBUTES, LPSECURITY_ATTRIBUTES, BOOL, DWORD,
                                       LPVOID, LPCWSTR, LPSTARTUPINFOW, LPPROCESS_INFORMATION);
typedef DWORD(WINAPI *GetConsoleProcessList_t)(LPDWORD, DWORD);
typedef BOOL(WINAPI *GetDiskFreeSpaceExW_t)(LPCWSTR, PULARGE_INTEGER, PULARGE_INTEGER, PULARGE_INTEGER);
typedef HINSTANCE(WINAPI *ShellExecuteW_t)(HWND, LPCWSTR, LPCWSTR, LPCWSTR, LPCWSTR, INT);
typedef LONG(WINAPI *RtlGetVersion_t)(OSVERSIONINFOW *);

static struct {
    CreateFileW_t CreateFileW;
    FindFirstFileW_t FindFirstFileW;
    FindNextFileW_t FindNextFileW;
    CreateDirectoryW_t CreateDirectoryW;
    GetFileAttributesW_t GetFileAttributesW;
    GetModuleFileNameW_t GetModuleFileNameW;
    GetCommandLineW_t GetCommandLineW;
    WriteConsoleW_t WriteConsoleW;
    ReadConsoleW_t ReadConsoleW;
    GetEnvironmentVariableW_t GetEnvironmentVariableW;
    CreateProcessW_t CreateProcessW;
    GetConsoleProcessList_t GetConsoleProcessList;
    GetDiskFreeSpaceExW_t GetDiskFreeSpaceExW;
} W;

static int g_nt;
static HANDLE g_heap, g_out, g_in;
static volatile LONG g_stop;
static DWORD g_open_error;

static void load_wide(void)
{
    HMODULE k = GetModuleHandleA("kernel32.dll");
#define LOAD(name) W.name = (name##_t)GetProcAddress(k, #name)
    LOAD(CreateFileW);
    LOAD(FindFirstFileW);
    LOAD(FindNextFileW);
    LOAD(CreateDirectoryW);
    LOAD(GetFileAttributesW);
    LOAD(GetModuleFileNameW);
    LOAD(GetCommandLineW);
    LOAD(WriteConsoleW);
    LOAD(ReadConsoleW);
    LOAD(GetEnvironmentVariableW);
    LOAD(CreateProcessW);
    LOAD(GetConsoleProcessList);
    LOAD(GetDiskFreeSpaceExW);
#undef LOAD
    /* every Unicode call we rely on must be there, or behave as Windows 9x */
    if (!W.CreateFileW || !W.FindFirstFileW || !W.FindNextFileW || !W.CreateDirectoryW ||
        !W.GetFileAttributesW || !W.GetModuleFileNameW || !W.GetCommandLineW)
        g_nt = 0;
}

/* ---- memory ---- */

void *mem_alloc(size_t n) { return HeapAlloc(g_heap, HEAP_ZERO_MEMORY, n ? n : 1); }

void *mem_realloc(void *p, size_t n)
{
    if (!p) return mem_alloc(n);
    return HeapReAlloc(g_heap, 0, p, n ? n : 1);
}

void mem_free(void *p)
{
    if (p) HeapFree(g_heap, 0, p);
}

/* ---- text conversion ---- */

static WCHAR *to_wide(const char *s, int extra)
{
    int n = MultiByteToWideChar(CP_UTF8, 0, s, -1, NULL, 0);
    WCHAR *w;
    if (n <= 0) n = 1;
    w = (WCHAR *)mem_alloc((size_t)(n + extra) * sizeof(WCHAR));
    if (w) MultiByteToWideChar(CP_UTF8, 0, s, -1, w + extra, n);
    return w;
}

static char *from_wide(const WCHAR *w)
{
    int n = WideCharToMultiByte(CP_UTF8, 0, w, -1, NULL, 0, NULL, NULL);
    char *s;
    if (n <= 0) return str_dup("");
    s = (char *)mem_alloc((size_t)n);
    if (s) WideCharToMultiByte(CP_UTF8, 0, w, -1, s, n, NULL, NULL);
    return s;
}

/* A path for the W functions: forward slashes turned round, and \\?\ in
 * front of drive paths so they may be longer than 260 characters. */
static WCHAR *wpath(const char *p)
{
    WCHAR *w, *q;
    int pre = 0, unc = 0;
    if (((p[0] | 32) >= 'a' && (p[0] | 32) <= 'z') && p[1] == ':' && (p[2] == '\\' || p[2] == '/')) pre = 4;
    else if ((p[0] == '\\' || p[0] == '/') && (p[1] == '\\' || p[1] == '/') && p[2] != '?' && p[2] != '.')
        pre = 8, unc = 1;
    w = to_wide(unc ? p + 2 : p, pre);
    if (!w) return NULL;
    if (pre) {
        const WCHAR *prefix = unc ? L"\\\\?\\UNC\\" : L"\\\\?\\";
        int i;
        for (i = 0; i < pre; i++) w[i] = prefix[i];
    }
    for (q = w; *q; q++)
        if (*q == '/') *q = '\\';
    return w;
}

static void ansi_path(char *dst, const char *p, int cap)
{
    int i;
    for (i = 0; p[i] && i < cap - 1; i++) dst[i] = p[i] == '/' ? '\\' : p[i];
    dst[i] = 0;
}

/* ---- console ---- */

static BOOL WINAPI on_ctrl(DWORD type)
{
    if (type == CTRL_C_EVENT || type == CTRL_BREAK_EVENT) {
        g_stop = 1;
        return TRUE;
    }
    return FALSE;
}

int stop_requested(void) { return g_stop != 0; }
void con_clear_stop(void) { g_stop = 0; }

int con_interactive(void)
{
    DWORD mode;
    return GetConsoleMode(g_out, &mode) != 0;
}

void con_write(const char *text)
{
    DWORD done;
    if (g_nt && W.WriteConsoleW && con_interactive()) {
        WCHAR *w = to_wide(text, 0);
        if (w) {
            DWORD n = 0;
            while (w[n]) n++;
            W.WriteConsoleW(g_out, w, n, &done, NULL);
            mem_free(w);
            return;
        }
    }
    WriteFile(g_out, text, (DWORD)str_len(text), &done, NULL);
}

int con_read_line(char *buf, int cap)
{
    DWORD got = 0, mode;
    int n = 0;
    if (g_nt && W.ReadConsoleW && GetConsoleMode(g_in, &mode)) {
        WCHAR w[1024];
        char *s;
        if (!W.ReadConsoleW(g_in, w, 1023, &got, NULL) || got == 0) return -1;
        w[got] = 0;
        s = from_wide(w);
        for (n = 0; s[n] && n < cap - 1; n++) buf[n] = s[n];
        buf[n] = 0;
        mem_free(s);
    } else { /* piped, or Windows 9x: a byte at a time, up to the newline */
        char c;
        for (;;) {
            if (!ReadFile(g_in, &c, 1, &got, NULL) || got == 0) {
                if (!n) return -1;
                break;
            }
            if (c == '\n') break;
            if (n < cap - 1) buf[n++] = c;
        }
        buf[n] = 0;
    }
    while (n > 0 && (buf[n - 1] == '\r' || buf[n - 1] == '\n')) buf[--n] = 0;
    return n;
}

int con_owned(void)
{
    DWORD ids[4];
    if (!W.GetConsoleProcessList) return 1; /* before XP: assume a double-click */
    return W.GetConsoleProcessList(ids, 4) <= 1;
}

/* ---- files ---- */

struct PFile {
    HANDLE h;
    int raw;
    u8 *bounce;
    u32 bounce_cap;
};

int path_is_raw(const char *p)
{
    return (p[0] == '\\' || p[0] == '/') && (p[1] == '\\' || p[1] == '/') && p[2] == '.' &&
           (p[3] == '\\' || p[3] == '/');
}

static HANDLE open_handle(const char *path, DWORD access, DWORD share, DWORD create)
{
    HANDLE h;
    if (path_is_raw(path)) { /* \\.\C: takes no \\?\ prefix */
        if (g_nt) {
            WCHAR *w = to_wide(path, 0);
            h = w ? W.CreateFileW(w, access, share, NULL, create, 0, NULL) : INVALID_HANDLE_VALUE;
            mem_free(w);
        } else {
            h = CreateFileA(path, access, share, NULL, create, 0, NULL);
        }
    } else if (g_nt) {
        WCHAR *w = wpath(path);
        h = w ? W.CreateFileW(w, access, share | FILE_SHARE_DELETE, NULL, create, FILE_FLAG_SEQUENTIAL_SCAN, NULL)
              : INVALID_HANDLE_VALUE;
        mem_free(w);
    } else {
        char a[MAX_PATH];
        ansi_path(a, path, MAX_PATH);
        h = CreateFileA(a, access, share, NULL, create, 0, NULL);
    }
    g_open_error = h == INVALID_HANDLE_VALUE ? GetLastError() : 0;
    return h;
}

PFile *f_open_read(const char *path)
{
    HANDLE h = open_handle(path, GENERIC_READ, FILE_SHARE_READ | FILE_SHARE_WRITE, OPEN_EXISTING);
    PFile *f;
    if (h == INVALID_HANDLE_VALUE) return NULL;
    f = (PFile *)mem_alloc(sizeof *f);
    if (!f) {
        CloseHandle(h);
        return NULL;
    }
    f->h = h;
    f->raw = path_is_raw(path);
    return f;
}

PFile *f_open_write(const char *path)
{
    HANDLE h = open_handle(path, GENERIC_WRITE, 0, CREATE_ALWAYS);
    PFile *f;
    if (h == INVALID_HANDLE_VALUE) return NULL;
    f = (PFile *)mem_alloc(sizeof *f);
    if (!f) {
        CloseHandle(h);
        return NULL;
    }
    f->h = h;
    return f;
}

int f_last_error_denied(void) { return g_open_error == ERROR_ACCESS_DENIED; }

static int seek(HANDLE h, u64 off)
{
    LONG hi = (LONG)(off >> 32);
    DWORD lo = SetFilePointer(h, (LONG)(u32)off, &hi, FILE_BEGIN);
    return lo == 0xFFFFFFFF && GetLastError() != NO_ERROR ? -1 : 0;
}

static int read_plain(HANDLE h, u64 off, u8 *buf, u32 n, u32 *got)
{
    *got = 0;
    if (seek(h, off)) return -1;
    while (*got < n) {
        DWORD r = 0;
        if (!ReadFile(h, buf + *got, n - *got, &r, NULL)) return *got ? 0 : -1;
        if (r == 0) break;
        *got += r;
    }
    return 0;
}

/* A raw disk can only be read in whole, aligned sectors. */
static int read_raw(PFile *f, u64 off, u8 *buf, u32 n, u32 *got)
{
    u64 base = off & ~(u64)4095;
    u32 skip = (u32)(off - base), total = (skip + n + 4095) & ~4095u, have = 0;
    *got = 0;
    if (total > f->bounce_cap) {
        if (f->bounce) VirtualFree(f->bounce, 0, MEM_RELEASE);
        f->bounce = (u8 *)VirtualAlloc(NULL, total, MEM_COMMIT, PAGE_READWRITE);
        f->bounce_cap = f->bounce ? total : 0;
        if (!f->bounce) return -1;
    }
    if (seek(f->h, base)) return -1;
    if (!ReadFile(f->h, f->bounce, total, (DWORD *)&have, NULL)) {
        /* the end of the disk: read what is left a sector at a time */
        have = 0;
        while (have < total) {
            DWORD r = 0;
            if (seek(f->h, base + have) || !ReadFile(f->h, f->bounce + have, 4096, &r, NULL) || r == 0) break;
            have += r;
        }
    }
    if (have <= skip) return have || !n ? 0 : -1;
    *got = have - skip < n ? have - skip : n;
    memcpy(buf, f->bounce + skip, *got);
    return 0;
}

int f_read_at(PFile *f, u64 off, void *buf, u32 n, u32 *got)
{
    return f->raw ? read_raw(f, off, (u8 *)buf, n, got) : read_plain(f->h, off, (u8 *)buf, n, got);
}

int f_write(PFile *f, const void *buf, u32 n)
{
    DWORD done = 0;
    return WriteFile(f->h, buf, n, &done, NULL) && done == n ? 0 : -1;
}

u64 f_size(PFile *f)
{
    DWORD hi = 0, lo;
    if (f->raw) {
        u8 info[8];
        DWORD r = 0;
        if (DeviceIoControl(f->h, 0x0007405C /* IOCTL_DISK_GET_LENGTH_INFO */, NULL, 0, info, sizeof info, &r, NULL))
            return rd_le64(info);
        return 0;
    }
    lo = GetFileSize(f->h, &hi);
    if (lo == 0xFFFFFFFF && GetLastError() != NO_ERROR) return 0;
    return ((u64)hi << 32) | lo;
}

void f_close(PFile *f)
{
    if (!f) return;
    CloseHandle(f->h);
    if (f->bounce) VirtualFree(f->bounce, 0, MEM_RELEASE);
    mem_free(f);
}

int dir_make(const char *path)
{
    BOOL ok;
    if (g_nt) {
        WCHAR *w = wpath(path);
        ok = w && W.CreateDirectoryW(w, NULL);
        mem_free(w);
    } else {
        char a[MAX_PATH];
        ansi_path(a, path, MAX_PATH);
        ok = CreateDirectoryA(a, NULL);
    }
    return ok || GetLastError() == ERROR_ALREADY_EXISTS ? 0 : -1;
}

int path_kind(const char *path)
{
    DWORD a;
    if (g_nt) {
        WCHAR *w = wpath(path);
        a = w ? W.GetFileAttributesW(w) : 0xFFFFFFFF;
        mem_free(w);
    } else {
        char p[MAX_PATH];
        ansi_path(p, path, MAX_PATH);
        a = GetFileAttributesA(p);
    }
    if (a == 0xFFFFFFFF) return -1;
    return (a & FILE_ATTRIBUTE_DIRECTORY) ? 1 : 0;
}

static void ft_date(const FILETIME *ft, Date *d)
{
    SYSTEMTIME st;
    memset(d, 0, sizeof *d);
    if (!ft->dwLowDateTime && !ft->dwHighDateTime) return;
    if (!FileTimeToSystemTime(ft, &st)) return;
    d->year = st.wYear;
    d->month = (u8)st.wMonth;
    d->day = (u8)st.wDay;
    d->hour = (u8)st.wHour;
    d->minute = (u8)st.wMinute;
    d->second = (u8)st.wSecond;
}

/* Cloud placeholders (OneDrive "online-only" files): opening one would
 * download it, so they are left alone. Reparse points are links and
 * junctions, which would mean visiting folders twice. */
#define SKIP_ATTRS (0x400 /* reparse point */ | 0x1000 /* offline */ | 0x40000 /* recall on open */ | 0x400000)

DirEntry *dir_list(const char *path, int *count)
{
    DirEntry *v = NULL;
    int n = 0, cap = 0;
    HANDLE h;
    WIN32_FIND_DATAW fw;
    WIN32_FIND_DATAA fa;
    char *pattern = path_join(path, "*");
    *count = 0;
    if (g_nt) {
        WCHAR *w = wpath(pattern);
        h = w ? W.FindFirstFileW(w, &fw) : INVALID_HANDLE_VALUE;
        mem_free(w);
    } else {
        char a[MAX_PATH];
        ansi_path(a, pattern, MAX_PATH);
        h = FindFirstFileA(a, &fa);
    }
    mem_free(pattern);
    if (h == INVALID_HANDLE_VALUE) return NULL;
    do {
        DWORD attrs = g_nt ? fw.dwFileAttributes : fa.dwFileAttributes;
        char *name = g_nt ? from_wide(fw.cFileName) : str_dup(fa.cFileName);
        DirEntry *e;
        if (!name) continue;
        if (str_eq(name, ".") || str_eq(name, "..") || (attrs & SKIP_ATTRS)) {
            mem_free(name);
            continue;
        }
        if (n == cap) {
            DirEntry *nv;
            cap = cap ? cap * 2 : 64;
            nv = (DirEntry *)mem_realloc(v, (size_t)cap * sizeof *nv);
            if (!nv) {
                mem_free(name);
                break;
            }
            v = nv;
        }
        e = &v[n++];
        e->name = name;
        e->is_dir = (attrs & FILE_ATTRIBUTE_DIRECTORY) != 0;
        if (g_nt) {
            e->size = ((u64)fw.nFileSizeHigh << 32) | fw.nFileSizeLow;
            ft_date(&fw.ftLastWriteTime, &e->mtime);
        } else {
            e->size = ((u64)fa.nFileSizeHigh << 32) | fa.nFileSizeLow;
            ft_date(&fa.ftLastWriteTime, &e->mtime);
        }
    } while (g_nt ? W.FindNextFileW(h, &fw) : FindNextFileA(h, &fa));
    FindClose(h);
    *count = n;
    if (!v) v = (DirEntry *)mem_alloc(sizeof *v);
    return v;
}

void dir_list_free(DirEntry *e, int count)
{
    int i;
    for (i = 0; i < count; i++) mem_free(e[i].name);
    mem_free(e);
}

char path_sep(void) { return '\\'; }

void filetime_to_date(u64 filetime, Date *d)
{
    FILETIME ft;
    ft.dwLowDateTime = (DWORD)filetime;
    ft.dwHighDateTime = (DWORD)(filetime >> 32);
    ft_date(&ft, d);
}

void date_now(Date *d)
{
    SYSTEMTIME st;
    GetLocalTime(&st);
    d->year = st.wYear;
    d->month = (u8)st.wMonth;
    d->day = (u8)st.wDay;
    d->hour = (u8)st.wHour;
    d->minute = (u8)st.wMinute;
    d->second = (u8)st.wSecond;
}

u32 ms_now(void) { return GetTickCount(); }

u32 random_seed(void)
{
    u32 x = GetTickCount() ^ (GetCurrentProcessId() << 16) ^ (u32)(size_t)&x;
    x ^= x >> 13;
    x *= 0x5bd1e995u;
    return x ^ (x >> 15);
}

/* ---- places ---- */

static int has_media(const char *root)
{
    char label[64];
    DWORD serial, maxlen, flags;
    return GetVolumeInformationA(root, label, sizeof label, &serial, &maxlen, &flags, NULL, 0) != 0;
}

char **search_roots(int *count)
{
    static char *roots[26];
    static char names[26][4];
    DWORD mask = GetLogicalDrives();
    int i, n = 0;
    for (i = 2; i < 26; i++) { /* A: and B: are floppy drives */
        UINT type;
        if (!(mask & (1u << i))) continue;
        names[n][0] = (char)('A' + i);
        names[n][1] = ':';
        names[n][2] = '\\';
        names[n][3] = 0;
        type = GetDriveTypeA(names[n]);
        if (type == DRIVE_FIXED || type == DRIVE_RAMDISK ||
            ((type == DRIVE_REMOVABLE || type == DRIVE_CDROM) && has_media(names[n]))) {
            roots[n] = names[n];
            n++;
        }
    }
    *count = n;
    return roots;
}

char **raw_volumes(int *count)
{
    static char *vols[26];
    static char names[26][8];
    DWORD mask = GetLogicalDrives();
    int i, n = 0;
    *count = 0;
    if (!g_nt) return vols;
    for (i = 2; i < 26; i++) {
        char root[4] = {(char)('A' + i), ':', '\\', 0};
        UINT type;
        if (!(mask & (1u << i))) continue;
        type = GetDriveTypeA(root);
        if (type != DRIVE_FIXED && !(type == DRIVE_REMOVABLE && has_media(root))) continue;
        memcpy(names[n], "\\\\.\\X:", 7);
        names[n][4] = (char)('A' + i);
        vols[n] = names[n];
        n++;
    }
    *count = n;
    return vols;
}

int raw_supported(void) { return g_nt; }

static char *g_exe_dir;

const char *exe_dir(void)
{
    if (!g_exe_dir) {
        char *p;
        size_t n;
        if (g_nt) {
            WCHAR w[1024];
            DWORD k = W.GetModuleFileNameW(NULL, w, 1023);
            w[k] = 0;
            p = from_wide(w);
        } else {
            char a[MAX_PATH];
            DWORD k = GetModuleFileNameA(NULL, a, MAX_PATH - 1);
            a[k] = 0;
            p = str_dup(a);
        }
        n = str_len(p);
        while (n && p[n - 1] != '\\' && p[n - 1] != '/') n--;
        if (n > 3) n--; /* keep C:\ whole */
        p[n] = 0;
        g_exe_dir = p;
    }
    return g_exe_dir;
}

const char *fallback_out_dir(void)
{
    static char *dir;
    if (dir) return dir;
    if (g_nt && W.GetEnvironmentVariableW) {
        WCHAR w[1024];
        DWORD n = W.GetEnvironmentVariableW(L"USERPROFILE", w, 1023);
        if (n && n < 1023) {
            char *home = from_wide(w), *desk = path_join(home, "Desktop");
            if (path_kind(desk) == 1) {
                mem_free(home);
                return dir = desk;
            }
            mem_free(desk);
            return dir = home;
        }
    } else {
        char a[MAX_PATH];
        UINT n = GetWindowsDirectoryA(a, MAX_PATH - 10);
        if (n && n < MAX_PATH - 10) {
            char *desk = path_join(a, "Desktop");
            if (path_kind(desk) == 1) return dir = desk;
            mem_free(desk);
        }
    }
    return dir = str_dup("C:\\");
}

const char *os_name(void)
{
    static char name[64];
    OSVERSIONINFOA v;
    const char *n = "Windows";
    DWORD major, minor, build;
    if (name[0]) return name;
    memset(&v, 0, sizeof v);
    v.dwOSVersionInfoSize = sizeof v;
    GetVersionExA(&v);
    major = v.dwMajorVersion;
    minor = v.dwMinorVersion;
    build = v.dwBuildNumber & 0xFFFF;
    if (v.dwPlatformId == VER_PLATFORM_WIN32_NT) { /* GetVersionEx lies after 8; ntdll does not */
        RtlGetVersion_t get = (RtlGetVersion_t)GetProcAddress(GetModuleHandleA("ntdll.dll"), "RtlGetVersion");
        OSVERSIONINFOW w;
        memset(&w, 0, sizeof w);
        w.dwOSVersionInfoSize = sizeof w;
        if (get && get(&w) == 0) {
            major = w.dwMajorVersion;
            minor = w.dwMinorVersion;
            build = w.dwBuildNumber;
        }
        if (major == 4) n = "Windows NT 4.0";
        else if (major == 5 && minor == 0) n = "Windows 2000";
        else if (major == 5 && minor == 1) n = "Windows XP";
        else if (major == 5) n = "Windows XP x64 / Server 2003";
        else if (major == 6 && minor == 0) n = "Windows Vista";
        else if (major == 6 && minor == 1) n = "Windows 7";
        else if (major == 6 && minor == 2) n = "Windows 8";
        else if (major == 6) n = "Windows 8.1";
        else if (major == 10 && build >= 22000) n = "Windows 11";
        else if (major == 10) n = "Windows 10";
    } else {
        if (minor == 0) n = "Windows 95";
        else if (minor == 10) n = "Windows 98";
        else n = "Windows ME";
    }
    memcpy(name, n, str_len(n) + 1);
    return name;
}

int open_folder(const char *path)
{
    char *cmd = fmt("explorer.exe \"%s\"", path);
    PROCESS_INFORMATION pi;
    BOOL ok;
    memset(&pi, 0, sizeof pi);
    if (g_nt && W.CreateProcessW) {
        STARTUPINFOW si;
        WCHAR *w = to_wide(cmd, 0);
        memset(&si, 0, sizeof si);
        si.cb = sizeof si;
        ok = w && W.CreateProcessW(NULL, w, NULL, NULL, FALSE, 0, NULL, NULL, &si, &pi);
        mem_free(w);
    } else {
        STARTUPINFOA si;
        memset(&si, 0, sizeof si);
        si.cb = sizeof si;
        ok = CreateProcessA(NULL, cmd, NULL, NULL, FALSE, 0, NULL, NULL, &si, &pi);
    }
    mem_free(cmd);
    if (!ok) return -1;
    CloseHandle(pi.hThread);
    CloseHandle(pi.hProcess);
    return 0;
}

int relaunch_elevated(const char *args)
{
    HMODULE shell;
    ShellExecuteW_t run;
    WCHAR exe[1024], *wargs;
    DWORD k;
    HINSTANCE r;
    if (!g_nt) return 0;
    shell = LoadLibraryA("shell32.dll");
    run = shell ? (ShellExecuteW_t)GetProcAddress(shell, "ShellExecuteW") : NULL;
    if (!run) return 0;
    k = W.GetModuleFileNameW(NULL, exe, 1023);
    exe[k] = 0;
    wargs = to_wide(args, 0);
    r = run(NULL, L"runas", exe, wargs, NULL, SW_SHOWNORMAL);
    mem_free(wargs);
    return (size_t)r > 32;
}

/* ---- start-up ---- */

/* The usual Windows rules for splitting a command line. */
static char **split_args(const char *cmd, int *argc)
{
    char **v = NULL;
    int n = 0;
    const char *p = cmd;
    for (;;) {
        Str a = {0};
        int quoted = 0;
        char **nv;
        while (*p == ' ' || *p == '\t') p++;
        if (!*p) break;
        while (*p && (quoted || (*p != ' ' && *p != '\t'))) {
            int slashes = 0;
            while (*p == '\\') slashes++, p++;
            if (*p == '"') {
                int i;
                for (i = 0; i < slashes / 2; i++) str_addc(&a, '\\');
                if (slashes % 2) str_addc(&a, '"');
                else quoted = !quoted;
                p++;
            } else {
                int i;
                for (i = 0; i < slashes; i++) str_addc(&a, '\\');
                if (*p && (quoted || (*p != ' ' && *p != '\t'))) str_addc(&a, *p++);
            }
        }
        nv = (char **)mem_realloc(v, (size_t)(n + 2) * sizeof(char *));
        if (!nv) break;
        v = nv;
        v[n++] = a.s ? a.s : str_dup("");
        v[n] = NULL;
    }
    *argc = n;
    return v;
}

void __cdecl ccf_entry(void)
{
    char **argv, *cmd;
    int argc = 0, r;
    g_heap = GetProcessHeap();
    g_nt = (GetVersion() & 0x80000000u) == 0;
    if (g_nt) load_wide();
    g_out = GetStdHandle(STD_OUTPUT_HANDLE);
    g_in = GetStdHandle(STD_INPUT_HANDLE);
    SetErrorMode(SEM_FAILCRITICALERRORS | SEM_NOOPENFILEERRORBOX);
    SetConsoleCtrlHandler(on_ctrl, TRUE);
    cmd = g_nt ? from_wide(W.GetCommandLineW()) : str_dup(GetCommandLineA());
    argv = split_args(cmd, &argc);
    r = app_main(argc, argv);
    ExitProcess((UINT)r);
}

#endif /* _WIN32 */
