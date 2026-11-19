#!/bin/bash
# run.sh: every idea in briefs.txt in every style in styles.txt, with Z-Image Turbo (enhance off, no LoRA).
# Needs the headless ComfyUI frontend the runner drives; see the workflows repo, tools/README.md. Output: out/<style>_<idea>_<seed>/
D=$(cd "$(dirname "$0")" && pwd); W=${COMFY_WORKFLOWS:?set COMFY_WORKFLOWS to homelab/services/butcher/comfyui/workflows}
cd $W/tools
while IFS='|' read -r sid style; do
  while IFS='|' read -r id subject; do
    for seed in ${SEEDS:-101 202}; do
      out=$D/out/${sid}_${id}_$seed
      [ -n "$(ls $out/*.png 2>/dev/null)" ] && continue
      mkdir -p $out
      python3 - "$D/patch.tmpl.js" "logo mark of $subject, $style, simple bold shapes that read at small size, centered with generous margin, no text, no letters, no watermark" "$seed" "1:1 (Square)" > $D/patch_cur.js <<'PY'
import json, sys
t = open(sys.argv[1]).read()
print(t.replace("__PROMPT__", json.dumps(sys.argv[2])).replace("__SEED__", sys.argv[3]).replace("__ASPECT__", json.dumps(sys.argv[4])))
PY
      start=$(date +%s)
      node run.mjs $W/image_zimage-turbo_t2i.json --patch $D/patch_cur.js --label isley-logo --out $out > $out.log 2>&1 < /dev/null
      echo "${sid}_${id} $seed: $(($(date +%s)-start))s $(ls $out/*.png 2>/dev/null | wc -l) png $(grep -o '"status": *"[a-z]*"' $out.log | head -1)"
    done
  done < $D/briefs.txt
done < $D/styles.txt
