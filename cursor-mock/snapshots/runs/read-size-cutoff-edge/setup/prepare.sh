#!/bin/sh
# Text files of exact sizes, deterministic: lines of 65 bytes, cut to the size.
for n in 10001 10100 10200 10239; do
  awk 'BEGIN{for(i=1;i<=1000;i++)printf "%05d abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ 0123\n", i}' | head -c "$n" >"f$n.txt"
done
