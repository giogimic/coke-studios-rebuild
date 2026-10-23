#ifndef CCF_HASH_H
#define CCF_HASH_H

#include "util.h"

void sha1(const u8 *data, u32 len, u8 out[20]);
void sha1_hex(const u8 digest[20], char out[41]);
u32 crc32(u32 crc, const u8 *p, u32 n);

#endif
