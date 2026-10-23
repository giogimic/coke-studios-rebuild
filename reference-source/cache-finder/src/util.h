/* Small helpers the C library would normally provide. The Windows build
 * links no C runtime (that is what lets it run on Windows 95), so strings,
 * formatting and growable buffers are all here. */
#ifndef CCF_UTIL_H
#define CCF_UTIL_H

#include "plat.h"

size_t str_len(const char *s);
char *str_dup(const char *s);
char *str_ndup(const char *s, size_t n);
int str_eq(const char *a, const char *b);
int str_ieq(const char *a, const char *b);
int str_istarts(const char *s, const char *prefix);
int str_iends(const char *s, const char *suffix);
const char *str_ifind(const char *hay, const char *needle);
const char *mem_ifind(const u8 *hay, u32 len, const char *needle);
int lower(int c);
char *path_join(const char *dir, const char *name);
const char *path_base(const char *p);
const char *path_ext(const char *p); /* after the last dot, or "" */

/* A growable string. Zero-initialise, then append. */
typedef struct {
    char *s;
    size_t len, cap;
} Str;
void str_add(Str *b, const char *s);
void str_addn(Str *b, const char *s, size_t n);
void str_addc(Str *b, char c);
void str_fmt(Str *b, const char *fmt, ...);
void str_vfmt(Str *b, const char *fmt, va_list ap);
void str_free(Str *b);
char *fmt(const char *fmt, ...); /* allocated; the caller frees */

/* %s %c %d %u (u32) %U (u64) %x %X (u32 hex), with an optional 0 flag and
 * width, plus %M - a u64 byte count as "1.5 MB" - and %N - a u64 with
 * thousands separators. */
void say(const char *fmt, ...);

/* A growable byte buffer. */
typedef struct {
    u8 *p;
    u32 len, cap;
} Buf;
int buf_reserve(Buf *b, u32 extra); /* 0 = ok */
int buf_add(Buf *b, const void *p, u32 n);
void buf_free(Buf *b);

u32 rd_le32(const u8 *p);
u16 rd_le16(const u8 *p);
u32 rd_be32(const u8 *p);
u64 rd_le64(const u8 *p);
void wr_le32(u8 *p, u32 v);
void wr_le16(u8 *p, u16 v);

void date_str(const Date *d, char out[11]);      /* 2005-03-14, or "" */
void datetime_str(const Date *d, char out[20]); /* 2005-03-14 21:07 */

/* Reads a whole file, up to cap bytes. Returns 0 on success. */
int read_file(const char *path, u32 cap, Buf *out, u64 *full_size);

#endif
