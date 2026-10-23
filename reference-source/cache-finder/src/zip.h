#ifndef CCF_ZIP_H
#define CCF_ZIP_H

#include "util.h"

typedef struct ZipWriter ZipWriter;
ZipWriter *zip_create(const char *path);
int zip_add(ZipWriter *z, const char *name, const u8 *data, u32 len, const Date *d);
int zip_close(ZipWriter *z); /* 0 = ok */

#endif
