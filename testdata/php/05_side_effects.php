<?php
$counter = 0;
function tick() {
    global $counter;
    $counter++;
    echo "tick $counter\n";
    return $counter;
}
function main() {
    global $counter;
    $arr = [0, 0, 0, 0, 0];
    $n = 0;
    $arr[$n++] = 7;
    $arr[$n++] = 8;
    $i = 0;
    while ($i++ + 0 < 3) {
        echo "loop $i\n";
    }
    if (tick() + 0 > 5) {
        echo "big\n";
    }
    $total = 0;
    $total += tick();
    $t = $n > 1 ? $n++ : 0;
    $st = [1, 2, 3, 4];
    while (count($st) && array_pop($st) > 2) {
        echo "popped\n";
    }
    $k = 0;
    for ($j = 0; $j < 3 && $k++ < 5; $j++) {
        echo "for $j $k\n";
    }
    echo $arr[0], " ", $arr[1], " ", $n, " ", $i, " ", $counter, " ", $total, " ", $t, " ", count($st), " ", $k, "\n";
}
main();
