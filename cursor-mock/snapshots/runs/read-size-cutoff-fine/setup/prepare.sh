#!/bin/sh
# Text files of exact sizes, deterministic. f<N>.txt: lines of 65 bytes, cut to N bytes.
# short<N>.txt: lines of 3 bytes ("ab" and a newline), cut to N bytes (many lines, few bytes each).
# long<N>.txt: lines of 1000 bytes, cut to N bytes (few lines).
for n in 9500 9800 10000 10240 10500 11000 11500; do
  awk 'BEGIN{for(i=1;i<=1000;i++)printf "%05d abcdefghijklmnopqrstuvwxyz ABCDEFGHIJKLMNOPQRSTUVWXYZ 0123\n", i}' | head -c "$n" >"f$n.txt"
done
awk 'BEGIN{for(i=1;i<=5000;i++)print "ab"}' | head -c 9249 >short9249.txt
awk 'BEGIN{for(i=1;i<=5000;i++)print "ab"}' | head -c 11000 >short11000.txt
awk 'BEGIN{for(i=1;i<=20;i++){s="";for(j=1;j<=99;j++)s=s "abcdefghi ";print s}}' | head -c 9249 >long9249.txt
awk 'BEGIN{for(i=1;i<=20;i++){s="";for(j=1;j<=99;j++)s=s "abcdefghi ";print s}}' | head -c 11000 >long11000.txt
