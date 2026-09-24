#!/usr/bin/env python3
"""Gabungkan INSERT per-baris (mysqldump --skip-extended-insert) menjadi
multi-row INSERT (cap ~LIMIT byte/statement).

Alasan: impor ke Aiven lewat TLS terikat RTT (~150ms/statement). Dengan
skip-extended-insert, ±100rb statement → berjam-jam. Setelah digabung jadi
paket kecil (~800KB), total hanya puluhan statement → impor < 1 menit.

Aman: setiap baris mysqldump adalah SATU statement lengkap berakhiran ';'
(string di-escape, tidak ada newline literal). Penggabungan hanya untuk
baris INSERT berurutan pada tabel yang sama, dan di-flush ulang saat
menemukan baris non-INSERT (LOCK/UNLOCK/CREATE dll) atau rusak.
"""
import sys

LIMIT = 800_000  # target maksimum byte per statement gabungan

def main(src, dst):
    cur_table = None
    buf, buf_bytes = [], 0
    max_stmt = 0
    orig_rows = merged_stmts = 0

    def flush():
        nonlocal buf, buf_bytes, merged_stmts, max_stmt
        if not buf:
            return
        merged_stmts += 1
        size = sum(len(b) for b in buf) + len(cur_table) + 30
        if size > max_stmt:
            max_stmt = size
        out.write("INSERT INTO `%s` VALUES\n%s;\n" % (cur_table, ",\n".join(buf)))
        buf, buf_bytes = [], 0

    with open(src, encoding="utf-8", errors="replace") as f, \
         open(dst, "w", encoding="utf-8") as out:
        for line in f:
            if line.startswith("INSERT INTO "):
                body = line.rstrip("\n")
                if not body.endswith(";"):
                    # baris INSERT tak lengkap → flush & teruskan apa adanya
                    flush()
                    out.write(line)
                    continue
                body = body[:-1]
                idx = body.rfind(" VALUES ")
                if idx < 0:
                    flush()
                    out.write(line)
                    continue
                prefix = body[:idx]  # "INSERT INTO `t`"
                vals = body[idx + len(" VALUES "):]  # "(...)"
                t = prefix.split("`")[1] if "`" in prefix else "?"
                orig_rows += 1
                if cur_table is None:
                    cur_table = t
                if t != cur_table or (buf and buf_bytes + len(vals) > LIMIT):
                    flush()
                    cur_table = t
                buf.append(vals)
                buf_bytes += len(vals)
            else:
                flush()
                out.write(line)
        flush()
    print(f"baris INSERT asli: {orig_rows} → statement gabungan: {merged_stmts} "
          f"(max {max_stmt} byte)")

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print("usage: merge_mysql_inserts.py <in.sql> <out.sql>", file=sys.stderr)
        sys.exit(2)
    main(sys.argv[1], sys.argv[2])