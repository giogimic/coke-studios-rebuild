# macOS and Linux. (Windows: run build.bat.)
#   make            ./CokeCacheFinder for this machine
#   make universal  a macOS binary for both Intel and Apple Silicon
CC ?= cc
CFLAGS ?= -O2 -Wall -Wextra -Wno-unused-parameter -std=c99
SRC = $(wildcard src/*.c)

CokeCacheFinder: $(SRC) src/*.h
	$(CC) $(CFLAGS) -o $@ $(SRC)

universal: $(SRC) src/*.h
	$(CC) $(CFLAGS) -arch x86_64 -arch arm64 -mmacosx-version-min=10.13 -o CokeCacheFinder $(SRC)

static: $(SRC) src/*.h
	$(CC) $(CFLAGS) -static -o CokeCacheFinder $(SRC)

clean:
	rm -f CokeCacheFinder

.PHONY: universal static clean
