<?php
// Input user tidak diketahui saat simulasi: GopherViz mengisinya dengan angka acak.
$n = (int) readline("Berapa bintang? ");
$m = (int) trim(fgets(STDIN));
for ($i = 1; $i <= $n % 4; $i++) {
    echo str_repeat("*", $i) . "\n";
}
echo $m > 50 ? "besar\n" : "kecil\n";
