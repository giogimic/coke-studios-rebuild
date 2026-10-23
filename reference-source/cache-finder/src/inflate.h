#ifndef CCF_INFLATE_H
#define CCF_INFLATE_H

#include "util.h"

/* Both return 0 on success, -1 when the input ran out, -2 when the output
 * reached maxout (what was decoded so far stays in out), -3 on bad data.
 * *used is the number of input bytes consumed. inflate_zlib checks the
 * Adler-32 trailer, which is what makes it safe to trust on carved data. */
int inflate_raw(const u8 *in, u32 inlen, Buf *out, u32 maxout, u32 *used);
int inflate_zlib(const u8 *in, u32 inlen, Buf *out, u32 maxout, u32 *used);
int zlib_header_ok(const u8 *p, u32 n);

#endif
