<?php
// Dasar PHP
function faktorial(int $n): int {
    if ($n <= 1) {
        return 1;
    }
    return $n * faktorial($n - 1);
}

$buah = ["apel", "jeruk", "mangga"];
foreach ($buah as $i => $nama) {
    echo "$i: $nama\n";
}

$stok = ['apel' => 3, 'jeruk' => 0];
foreach ($stok as $nama => $jumlah) {
    if ($jumlah > 0) {
        echo "{$nama} tersedia ($jumlah)\n";
    } elseif ($jumlah == 0) {
        echo $nama . " habis\n";
    } else {
        echo "aneh\n";
    }
}

for ($i = 0; $i < 3; $i++) {
    echo $i;
}

$n = 5;
while ($n > 0) {
    $n -= 2;
}

switch ($n) {
    case -1:
        echo "minus satu";
        break;
    default:
        echo "lain";
}

class Akun {
    private float $saldo = 0;
    public static int $jumlah = 0;

    public function __construct(public string $nama) {
        self::$jumlah++;
    }

    public function setor(float $x): void {
        $this->saldo += $x;
    }

    public function saldo(): float {
        return $this->saldo;
    }
}

$a = new Akun("Budi");
$a->setor(100);
$label = match (true) {
    $a->saldo() > 50 => "kaya",
    default => "biasa",
};
echo faktorial(5), " ", $label, " ", Akun::$jumlah, PHP_EOL;
?>
<p>HTML di luar PHP</p>
