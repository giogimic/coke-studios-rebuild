/* The platform layer: everything the finder needs from the operating
 * system, and nothing else. There are two of these - plat_win32.c (Windows
 * 95 to 11, no C runtime at all) and plat_posix.c (macOS and Linux) - and
 * the rest of the program is written only against this header, which is
 * what lets one Windows .exe run on a 1998 PC and a 2026 one alike.
 *
 * Paths are UTF-8 strings everywhere except on Windows 95/98/ME, where they
 * are the system's own ANSI bytes, passed through untouched. */
#ifndef CCF_PLAT_H
#define CCF_PLAT_H

#include <stddef.h>
#include <stdarg.h>
#include <string.h> /* memcpy, memset, memcmp, memmove: plat_win32.c supplies them */

typedef unsigned char u8;
typedef unsigned short u16;
typedef unsigned int u32;
typedef int i32;
typedef unsigned long long u64;
typedef long long i64;

/* A calendar date and time. year == 0 means "unknown". */
typedef struct {
    u16 year;
    u8 month, day, hour, minute, second;
} Date;

/* Memory. mem_alloc returns zeroed memory or NULL. */
void *mem_alloc(size_t n);
void *mem_realloc(void *p, size_t n);
void mem_free(void *p);

/* The console. */
void con_write(const char *text);
int con_read_line(char *buf, int cap); /* -1 when there is no input */
int con_owned(void);                   /* the window closes when we exit */
int con_interactive(void);             /* output is a screen, not a file */
int stop_requested(void);              /* Ctrl+C was pressed */
void con_clear_stop(void);

/* Files. A raw device (\\.\C:, /dev/disk2) is opened the same way and read
 * through the same call; f_read_at deals with sector alignment. */
typedef struct PFile PFile;
PFile *f_open_read(const char *path);
PFile *f_open_write(const char *path);
int f_read_at(PFile *f, u64 off, void *buf, u32 n, u32 *got); /* 0 = ok */
int f_write(PFile *f, const void *buf, u32 n);                /* 0 = ok */
u64 f_size(PFile *f); /* 0 when unknown (some devices) */
void f_close(PFile *f);
int f_last_error_denied(void); /* the last open failed for lack of rights */

int dir_make(const char *path);    /* 0 = ok or already there */
int path_kind(const char *path);   /* 1 folder, 0 file, -1 missing */
int path_is_raw(const char *path); /* \\.\X: or /dev/... */

typedef struct {
    char *name;
    int is_dir;
    u64 size;
    Date mtime; /* UTC */
} DirEntry;
DirEntry *dir_list(const char *path, int *count); /* NULL when unreadable */
void dir_list_free(DirEntry *e, int count);

char path_sep(void);

/* Time. */
void filetime_to_date(u64 filetime, Date *d); /* Windows FILETIME, UTC */
void date_now(Date *d);                       /* local */
u32 ms_now(void);
u32 random_seed(void);

/* Places. */
char **search_roots(int *count); /* every drive/volume worth searching */
char **raw_volumes(int *count);  /* readable as raw devices, for deleted files */
const char *exe_dir(void);
const char *fallback_out_dir(void); /* desktop or home, when exe_dir is read-only */
const char *os_name(void);
int open_folder(const char *path);
int relaunch_elevated(const char *args); /* 1 = a new elevated copy is starting */
int raw_supported(void);

/* The program itself (main.c). */
int app_main(int argc, char **argv);

#endif
