#include "util.h"

size_t str_len(const char *s)
{
    size_t n = 0;
    if (!s) return 0;
    while (s[n]) n++;
    return n;
}

char *str_ndup(const char *s, size_t n)
{
    char *d = (char *)mem_alloc(n + 1);
    if (!d) return NULL;
    if (n) memcpy(d, s, n);
    d[n] = 0;
    return d;
}

char *str_dup(const char *s)
{
    return str_ndup(s ? s : "", str_len(s));
}

int lower(int c)
{
    return (c >= 'A' && c <= 'Z') ? c + 32 : c;
}

int str_eq(const char *a, const char *b)
{
    while (*a && *a == *b) a++, b++;
    return *a == *b;
}

int str_ieq(const char *a, const char *b)
{
    while (*a && lower((u8)*a) == lower((u8)*b)) a++, b++;
    return lower((u8)*a) == lower((u8)*b);
}

int str_istarts(const char *s, const char *prefix)
{
    while (*prefix) {
        if (lower((u8)*s) != lower((u8)*prefix)) return 0;
        s++, prefix++;
    }
    return 1;
}

int str_iends(const char *s, const char *suffix)
{
    size_t a = str_len(s), b = str_len(suffix);
    return a >= b && str_ieq(s + a - b, suffix);
}

const char *str_ifind(const char *hay, const char *needle)
{
    if (!*needle) return hay;
    for (; *hay; hay++)
        if (lower((u8)*hay) == lower((u8)*needle) && str_istarts(hay, needle)) return hay;
    return NULL;
}

/* Case-insensitive search in binary data; needle must be lowercase ASCII. */
const char *mem_ifind(const u8 *hay, u32 len, const char *needle)
{
    u32 n = (u32)str_len(needle), i, j;
    u8 first = (u8)needle[0];
    if (n == 0 || len < n) return NULL;
    for (i = 0; i + n <= len; i++) {
        if (lower(hay[i]) != first) continue;
        for (j = 1; j < n && lower(hay[i + j]) == (u8)needle[j]; j++) {}
        if (j == n) return (const char *)hay + i;
    }
    return NULL;
}

char *path_join(const char *dir, const char *name)
{
    Str b = {0};
    size_t n = str_len(dir);
    str_add(&b, dir);
    if (n && dir[n - 1] != '\\' && dir[n - 1] != '/') str_addc(&b, path_sep());
    str_add(&b, name);
    return b.s;
}

const char *path_base(const char *p)
{
    const char *b = p;
    for (; *p; p++)
        if (*p == '\\' || *p == '/') b = p + 1;
    return b;
}

const char *path_ext(const char *p)
{
    const char *b = path_base(p), *dot = NULL;
    for (; *b; b++)
        if (*b == '.') dot = b;
    return dot ? dot + 1 : "";
}

/* ---- growable strings ---- */

static int str_grow(Str *b, size_t extra)
{
    size_t want = b->len + extra + 1;
    char *n;
    if (want <= b->cap) return 0;
    if (want < b->cap * 2) want = b->cap * 2;
    if (want < 64) want = 64;
    n = (char *)mem_realloc(b->s, want);
    if (!n) return -1;
    b->s = n;
    b->cap = want;
    return 0;
}

void str_addn(Str *b, const char *s, size_t n)
{
    if (str_grow(b, n)) return;
    if (n) memcpy(b->s + b->len, s, n);
    b->len += n;
    b->s[b->len] = 0;
}

void str_add(Str *b, const char *s)
{
    str_addn(b, s ? s : "(null)", str_len(s ? s : "(null)"));
}

void str_addc(Str *b, char c)
{
    str_addn(b, &c, 1);
}

void str_free(Str *b)
{
    mem_free(b->s);
    b->s = NULL;
    b->len = b->cap = 0;
}

/* u64 / 10 in 32-bit pieces: the Windows build has no runtime helper for
 * 64-bit division. */
static u32 divmod10(u64 *v)
{
    u32 hi = (u32)(*v >> 32), lo = (u32)*v;
    u32 qhi = hi / 10, r = hi % 10, x, q1, q0;
    x = (r << 16) | (lo >> 16);
    q1 = x / 10;
    r = x % 10;
    x = (r << 16) | (lo & 0xFFFF);
    q0 = x / 10;
    r = x % 10;
    *v = ((u64)qhi << 32) | (u64)((q1 << 16) | q0);
    return r;
}

static void add_padded(Str *b, const char *digits, int width, char pad)
{
    int n = (int)str_len(digits);
    while (n < width) str_addc(b, pad), width--;
    str_add(b, digits);
}

static void fmt_u64(char *out, u64 v, int commas)
{
    char tmp[32];
    int n = 0, i = 0, k = 0;
    do {
        if (commas && k == 3) tmp[n++] = ',', k = 0;
        tmp[n++] = (char)('0' + divmod10(&v));
        k++;
    } while (v);
    while (n) out[i++] = tmp[--n];
    out[i] = 0;
}

static void fmt_hex(char *out, u32 v, int upper)
{
    const char *digits = upper ? "0123456789ABCDEF" : "0123456789abcdef";
    char tmp[9];
    int n = 0, i = 0;
    do {
        tmp[n++] = digits[v & 15];
        v >>= 4;
    } while (v);
    while (n) out[i++] = tmp[--n];
    out[i] = 0;
}

static void fmt_bytes(char *out, u64 v)
{
    Str b = {0};
    char num[32];
    if (v < 1024) {
        fmt_u64(num, v, 0);
        str_add(&b, num);
        str_add(&b, v == 1 ? " byte" : " bytes");
    } else if (v < ((u64)1 << 20)) {
        fmt_u64(num, (v + 512) >> 10, 0);
        str_add(&b, num);
        str_add(&b, " KB");
    } else {
        int gb = v >= ((u64)1 << 30);
        u32 units = gb ? (u32)(v >> 20) : (u32)(v >> 10); /* MB or KB */
        u32 tenths = units / 1024 * 10 + (units % 1024) * 10 / 1024;
        fmt_u64(num, tenths / 10, 1);
        str_add(&b, num);
        str_addc(&b, '.');
        str_addc(&b, (char)('0' + tenths % 10));
        str_add(&b, gb ? " GB" : " MB");
    }
    memcpy(out, b.s, b.len + 1);
    str_free(&b);
}

void str_vfmt(Str *b, const char *f, va_list ap)
{
    char num[48];
    for (; *f; f++) {
        int width = 0;
        char pad = ' ';
        if (*f != '%') {
            const char *start = f;
            while (f[1] && f[1] != '%') f++;
            str_addn(b, start, (size_t)(f - start + 1));
            continue;
        }
        f++;
        if (*f == '0') pad = '0', f++;
        while (*f >= '0' && *f <= '9') width = width * 10 + (*f++ - '0');
        switch (*f) {
        case 's': {
            const char *s = va_arg(ap, const char *);
            int n = (int)str_len(s ? s : "");
            while (n < width) str_addc(b, ' '), width--;
            str_add(b, s ? s : "");
            break;
        }
        case 'c': str_addc(b, (char)va_arg(ap, int)); break;
        case 'd': {
            int v = va_arg(ap, int);
            u64 u = v < 0 ? (u64)(u32)(-v) : (u64)(u32)v;
            fmt_u64(num + 1, u, 0);
            if (v < 0) {
                num[0] = '-';
                add_padded(b, num, width, pad);
            } else
                add_padded(b, num + 1, width, pad);
            break;
        }
        case 'u': fmt_u64(num, (u64)va_arg(ap, u32), 0); add_padded(b, num, width, pad); break;
        case 'U': fmt_u64(num, va_arg(ap, u64), 0); add_padded(b, num, width, pad); break;
        case 'N': fmt_u64(num, va_arg(ap, u64), 1); add_padded(b, num, width, pad); break;
        case 'x': fmt_hex(num, va_arg(ap, u32), 0); add_padded(b, num, width, pad); break;
        case 'X': fmt_hex(num, va_arg(ap, u32), 1); add_padded(b, num, width, pad); break;
        case 'M': fmt_bytes(num, va_arg(ap, u64)); add_padded(b, num, width, pad); break;
        case '%': str_addc(b, '%'); break;
        case 0: return;
        default: str_addc(b, '%'); str_addc(b, *f); break;
        }
    }
}

void str_fmt(Str *b, const char *f, ...)
{
    va_list ap;
    va_start(ap, f);
    str_vfmt(b, f, ap);
    va_end(ap);
}

char *fmt(const char *f, ...)
{
    Str b = {0};
    va_list ap;
    va_start(ap, f);
    str_vfmt(&b, f, ap);
    va_end(ap);
    if (!b.s) return str_dup("");
    return b.s;
}

void say(const char *f, ...)
{
    Str b = {0};
    va_list ap;
    va_start(ap, f);
    str_vfmt(&b, f, ap);
    va_end(ap);
    if (b.s) con_write(b.s);
    str_free(&b);
}

/* ---- byte buffers ---- */

int buf_reserve(Buf *b, u32 extra)
{
    u32 want = b->len + extra;
    u8 *n;
    if (want < b->len) return -1; /* overflow */
    if (want <= b->cap) return 0;
    if (want < b->cap + b->cap / 2) want = b->cap + b->cap / 2;
    if (want < 4096) want = 4096;
    n = (u8 *)mem_realloc(b->p, want);
    if (!n) return -1;
    b->p = n;
    b->cap = want;
    return 0;
}

int buf_add(Buf *b, const void *p, u32 n)
{
    if (buf_reserve(b, n)) return -1;
    memcpy(b->p + b->len, p, n);
    b->len += n;
    return 0;
}

void buf_free(Buf *b)
{
    mem_free(b->p);
    b->p = NULL;
    b->len = b->cap = 0;
}

u32 rd_le32(const u8 *p) { return (u32)p[0] | ((u32)p[1] << 8) | ((u32)p[2] << 16) | ((u32)p[3] << 24); }
u16 rd_le16(const u8 *p) { return (u16)(p[0] | (p[1] << 8)); }
u32 rd_be32(const u8 *p) { return ((u32)p[0] << 24) | ((u32)p[1] << 16) | ((u32)p[2] << 8) | (u32)p[3]; }
u64 rd_le64(const u8 *p) { return (u64)rd_le32(p) | ((u64)rd_le32(p + 4) << 32); }
void wr_le32(u8 *p, u32 v) { p[0] = (u8)v; p[1] = (u8)(v >> 8); p[2] = (u8)(v >> 16); p[3] = (u8)(v >> 24); }
void wr_le16(u8 *p, u16 v) { p[0] = (u8)v; p[1] = (u8)(v >> 8); }

static void two(char *o, u32 v) { o[0] = (char)('0' + v / 10 % 10); o[1] = (char)('0' + v % 10); }

void date_str(const Date *d, char out[11])
{
    if (!d || !d->year) {
        out[0] = 0;
        return;
    }
    two(out, d->year / 100);
    two(out + 2, d->year % 100);
    out[4] = '-';
    two(out + 5, d->month);
    out[7] = '-';
    two(out + 8, d->day);
    out[10] = 0;
}

void datetime_str(const Date *d, char out[20])
{
    date_str(d, out);
    if (!out[0]) return;
    out[10] = ' ';
    two(out + 11, d->hour);
    out[13] = ':';
    two(out + 14, d->minute);
    out[16] = 0;
}

int read_file(const char *path, u32 cap, Buf *out, u64 *full_size)
{
    PFile *f = f_open_read(path);
    u64 size;
    u32 want, got = 0;
    out->len = 0;
    if (!f) return -1;
    size = f_size(f);
    if (full_size) *full_size = size;
    want = size > cap ? cap : (u32)size;
    if (buf_reserve(out, want + 1)) {
        f_close(f);
        return -1;
    }
    if (want && f_read_at(f, 0, out->p, want, &got)) {
        f_close(f);
        return -1;
    }
    out->len = got;
    f_close(f);
    return 0;
}
