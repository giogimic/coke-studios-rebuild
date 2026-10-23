/* The finder's own vocabulary: what a found file is, how it was found and
 * what the search has collected so far. */
#ifndef CCF_FINDER_H
#define CCF_FINDER_H

#include "util.h"

#define CCF_VERSION "1.0"
/* Where people are told to send their results. Set this before a release. */
#define CCF_PROJECT_URL "https://github.com/Lost-Media-Hunt/Coke-Studios"

#define MAX_FILE (128u << 20) /* nothing the game shipped was anywhere near this */

/* ---- recognising files (formats.c) ---- */

enum { K_NONE, K_DIRECTOR, K_SWF, K_TEXT };

typedef struct {
    int kind;
    const char *what; /* "Shockwave cast" */
    const char *ext;  /* "cct" */
    u64 length;       /* the length its header declares (uncompressed for CWS) */
    int afterburner;  /* a compressed Shockwave file (.cct/.dcr) */
    int cws;          /* a compressed Flash movie */
    int version;      /* Flash version */
    char codec[5];    /* Director's container type, e.g. FGDC */
} Sniff;

/* Looks at the first bytes of something. strict is for bytes found in the
 * middle of a bigger file, where a chance match must be ruled out. */
int sniff(const u8 *p, u32 n, Sniff *s, int strict);

typedef struct {
    u32 length;  /* how much of the input is this file */
    int damaged; /* cut short, or the compressed parts do not decode */
    Buf text;    /* everything decompressed from inside it, for the word search */
} Exam;

/* Works out a file's true extent and unpacks what it can of it. Returns 0
 * when strict and the bytes turn out not to be a file after all. */
int examine(const u8 *p, u32 n, const Sniff *s, int strict, Exam *x);
void exam_free(Exam *x);

/* ---- deciding what is Coke Studios (coke.c) ---- */

typedef struct {
    int strong, weak;
    Str words; /* which ones, for the report */
} Markers;
void find_markers(const u8 *p, u32 n, Markers *m);
void markers_free(Markers *m);
int coke_url(const char *url);              /* the address is one of the game's sites */
void url_host(const char *url, char *out, int cap);
const char *wanted_label(const char *name, const char *url);
int name_identifies(const char *name);       /* a textless lost file known by its name alone */
/* What a file is, read from its own contents (for carved, nameless finds).
 * Sets *out_wanted when the contents say it is a lost room. */
const char *identify_content(const u8 *p, u32 n, const u8 *text, u32 tn, const char **out_wanted);
char *label_slug(const char *label); /* a label as a file-name stem */
int name_is_cokeish(const char *name);
int text_name_wanted(const char *name);
int cachey_dir(const char *name);
int match_host_at(const u8 *p, u32 avail); /* length of a Coke host word at p, or 0 */
int known_text_marker(const u8 *p, u32 n);
const char *known_name(const char *sha1hex, u64 size);
int known_count(void);

/* ---- what the search found (findings.c) ---- */

enum { CAT_COKE = 1, CAT_OTHER = 2 };

typedef struct {
    int cat;
    char *name;   /* the name it is saved under */
    char *saved;  /* path inside the results ("files/coke/x.cct"), NULL if not copied */
    char *source; /* where it was, with the user's name taken out */
    char *url;
    const char *what;
    char *why;
    const char *wanted;
    const char *identified; /* what its own contents say it is, or NULL */
    const char *known;      /* its name in the archive, or NULL: new */
    char sha1[41];
    u64 size;
    Date file_date, server_date;
    int damaged, carved, copies;
} Finding;

typedef struct {
    char *text;
    Date date;
    int count;
} Note;

typedef struct {
    Note *v;
    int n, cap;
} Notes;

typedef struct {
    char *out_base; /* folder the results folder goes in */
    char *out_dir;  /* the results folder, created when first needed */
    char *out_name;
    Finding *f;
    int nf, capf;
    Notes urls, notes, named;
    char **done;
    int ndone, capdone;
    char **places;
    int nplaces;
    u64 files_checked, dirs_checked, bytes_read;
    u32 start_ms, status_ms;
    int status_shown;
    int deleted_mode, stopped;
    int dry_run, verbose; /* maintainer modes */
} Scan;

typedef struct {
    const u8 *data;
    u32 len;
    const char *path; /* the file it came from */
    u64 offset;       /* where in that file, when carved */
    int carved;
    const char *name; /* a name for it */
    const char *url;
    Date file_date, server_date;
    int text_hint; /* a config-file name worth reading as text */
} Blob;

void consider(Scan *s, const Blob *b);
const char *url_start(const char *s);
void add_url(Scan *s, const char *url, const Date *d);
void add_note(Scan *s, const char *text, const Date *d);
void add_named(Scan *s, const char *path, u64 size, const Date *d);
void urls_in(Scan *s, const u8 *p, u32 n, const Date *d);
int url_at(Scan *s, const u8 *p, u32 i, u32 n, const Date *d); /* returns bytes to skip */
char *redact(const char *path);
void status(Scan *s, const char *what, const char *path);
void status_clear(Scan *s);
void mark_done(Scan *s, const char *path);
int is_done(Scan *s, const char *path);
void date_from_unix(u32 t, Date *d);

/* ---- searching (scan.c) ---- */

void scan_tree(Scan *s, const char *root);
void scan_path(Scan *s, const char *path);
int scan_raw(Scan *s, const char *device);
void carve(Scan *s, const char *path, PFile *f, u64 size, const Date *d, int raw);

/* ---- browser caches (iecache.c, ffcache.c) ---- */

void ie_index(Scan *s, const char *dir);
void ff_cache(Scan *s, const char *dir);

/* ---- results (report.c) ---- */

int ensure_out_dir(Scan *s);
int write_results(Scan *s, int make_zip, char **zip_path);
int make_known(const char *out_path, char **dirs, int ndirs);

#endif
