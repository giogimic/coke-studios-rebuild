#ifndef CCF_KNOWN_H
#define CCF_KNOWN_H

#include "plat.h"

/* Every file already in the project's archive, by SHA-1, so a find can be
 * told apart from something the project has had for years. known.c is
 * generated: CokeCacheFinder --make-known src/known.c <archive folders>. */
typedef struct {
    const char *sha1;
    u32 size;
    const char *name;
} KnownFile;

extern const KnownFile KNOWN_FILES[];
extern const int KNOWN_FILES_COUNT;

#endif
