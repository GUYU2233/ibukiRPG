#!/usr/bin/env bash
# 检查 APK 是否满足 Android 15+ 16 KB 页大小要求：
#   1. lib/*/*.so 的每个 ELF PT_LOAD 段 p_align >= 16384（默认检查全部 ABI；
#      REQUIRED_ABIS 中的 ABI 不满足即失败，其余 ABI 只告警。默认全部必需）
#   2. zipalign -c -P 16 -v 4 通过（未压缩 .so 在 ZIP 内按 16 KB 对齐）
# 用法：tools/check-apk-alignment.sh app.apk
# 环境变量：ZIPALIGN（默认 $ANDROID_HOME/build-tools/<最新>/zipalign）、REQUIRED_ABIS（如 "arm64-v8a x86_64"；默认 all）
set -euo pipefail
apk="${1:?usage: $0 <apk>}"
required="${REQUIRED_ABIS:-all}"

python3 - "$apk" "$required" <<'PY'
import struct, sys, zipfile
apk, required = sys.argv[1], sys.argv[2].split()
PT_LOAD = 1
bad_required, bad_other, n = [], [], 0
with zipfile.ZipFile(apk) as z:
    for info in z.infolist():
        name = info.filename
        if not (name.startswith("lib/") and name.endswith(".so")):
            continue
        n += 1
        abi = name.split("/")[1]
        data = z.read(info)
        if data[:4] != b"\x7fELF":
            print(f"FAIL {name}: not an ELF file"); bad_required.append(name); continue
        is64 = data[4] == 2
        end = "<" if data[5] == 1 else ">"
        if is64:
            phoff, = struct.unpack_from(end + "Q", data, 0x20)
            phentsize, phnum = struct.unpack_from(end + "HH", data, 0x36)
        else:
            phoff, = struct.unpack_from(end + "I", data, 0x1C)
            phentsize, phnum = struct.unpack_from(end + "HH", data, 0x2A)
        aligns = []
        for i in range(phnum):
            off = phoff + i * phentsize
            p_type, = struct.unpack_from(end + "I", data, off)
            if p_type != PT_LOAD:
                continue
            p_align, = struct.unpack_from(end + ("Q" if is64 else "I"), data, off + (0x30 if is64 else 0x1C))
            aligns.append(p_align)
        min_align = min(aligns) if aligns else 0
        ok = min_align >= 16384
        status = "OK  " if ok else "FAIL"
        print(f"{status} {name}: LOAD p_align={sorted(set(aligns))}")
        if not ok:
            (bad_required if ("all" in required or abi in required) else bad_other).append(name)
if n == 0:
    print("FAIL: no native libraries found in APK"); sys.exit(1)
for b in bad_other:
    print(f"WARNING (not required): {b} is not 16 KB aligned")
if bad_required:
    print(f"FAIL: {len(bad_required)} native librar(y/ies) with p_align < 16384: {bad_required}")
    sys.exit(1)
print(f"ELF alignment OK: {n} .so files")
PY

za="${ZIPALIGN:-}"
if [ -z "$za" ]; then
  if command -v zipalign >/dev/null 2>&1; then za=zipalign
  else za=$(ls -d "${ANDROID_HOME:-${ANDROID_SDK_ROOT:-/opt/android-sdk}}"/build-tools/*/zipalign 2>/dev/null | sort -V | tail -1)
  fi
fi
[ -n "$za" ] || { echo "FAIL: zipalign not found"; exit 1; }
if out=$("$za" -c -P 16 -v 4 "$apk" 2>&1); then
  echo "zipalign -c -P 16 -v 4: OK"
else
  echo "$out" | grep -v '(OK' || true
  echo "FAIL: zipalign -c -P 16 -v 4 failed"; exit 1
fi
