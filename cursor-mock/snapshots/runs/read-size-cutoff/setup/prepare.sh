#!/bin/sh
# Text files of exact sizes, deterministic: lines of 70 bytes, cut to the size.
for n in 7602 8000 9249 12000 16000 24000 32000 48000; do
  awk 'BEGIN{for(i=1;i<=1000;i++)printf "%05d abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ 0123\n", i}' | head -c "$n" >"f$n.txt"
done
