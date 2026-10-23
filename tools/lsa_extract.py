#!/usr/bin/env python3
"""Extract WAV clips from an ABBYY Lingvo .lsa sound archive (format ported from lsd2dsl LSAReader)."""
import io, os, struct, sys
import numpy as np
import soundfile as sf


def read_str(f):
    out = []
    while True:
        a = f.read(1)
        if not a or a[0] == 0xFF:
            break
        b = f.read(1)
        if b[0] == 0xFF:
            break
        out.append(chr(a[0] | (b[0] << 8)))
    return "".join(out)


def read_index(f):
    if read_str(f) != "L9SA":
        raise ValueError("not an LSA archive")
    (count,) = struct.unpack("<I", f.read(4))
    entries = []
    for i in range(count):
        name = read_str(f).strip()
        offset = 0
        if i > 0:
            (offset,) = struct.unpack("<I", f.read(4))
            marker = f.read(1)[0]
            if marker == 0:  # group header, no audio
                continue
            if marker != 0xFF:
                raise ValueError(f"bad LSA marker at entry {i}")
        (size,) = struct.unpack("<I", f.read(4))
        entries.append((name, offset, size))
    return entries, f.tell()


def main(lsa, outdir, limit=None):
    with open(lsa, "rb") as f:
        entries, ogg_off = read_index(f)
        f.seek(ogg_off)
        ogg = f.read()
    print(f"{len(entries)} clips; ogg stream at 0x{ogg_off:x}")
    os.makedirs(outdir, exist_ok=True)
    with sf.SoundFile(io.BytesIO(ogg)) as snd:
        rate, ch = snd.samplerate, snd.channels
        for n, (name, offset, size) in enumerate(entries[:limit]):
            snd.seek(offset)
            data = snd.read(size, dtype="int16")
            sf.write(os.path.join(outdir, name.replace("/", "_")), data, rate, subtype="PCM_16")
    print("done")


if __name__ == "__main__":
    if len(sys.argv) < 3:
        sys.exit("usage: lsa_extract.py ARCHIVE.lsa OUTDIR [LIMIT]")
    main(sys.argv[1], sys.argv[2], int(sys.argv[3]) if len(sys.argv) > 3 else None)
