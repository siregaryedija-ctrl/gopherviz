<?php
declare(strict_types=1);

interface Bentuk {
    public function luas(): float;
}

abstract class BentukDasar implements Bentuk {
    protected static int $jumlah = 0;
    const SATUAN = "cm";

    public function __construct(protected string $nama) {
        static::$jumlah++;
    }

    abstract public function luas(): float;

    public function __toString(): string {
        return sprintf("%s: %.2f %s²", $this->nama, $this->luas(), self::SATUAN);
    }

    public static function jumlah(): int {
        return self::$jumlah;
    }
}

class Lingkaran extends BentukDasar {
    public function __construct(private float $r) {
        parent::__construct("lingkaran");
    }
    public function luas(): float {
        return M_PI * $this->r ** 2;
    }
}

class Persegi extends BentukDasar {
    public function __construct(private float $sisi) {
        parent::__construct("persegi");
    }
    public function luas(): float {
        return $this->sisi * $this->sisi;
    }
}

$bentuk = [new Lingkaran(2), new Persegi(3), new Lingkaran(1)];
usort($bentuk, fn($a, $b) => $a->luas() <=> $b->luas());
foreach ($bentuk as $b) {
    echo $b, PHP_EOL;
}
echo "total bentuk: " . BentukDasar::jumlah() . "\n";

class SaldoTidakCukup extends Exception {
    public function __construct(private int $kurang) {
        parent::__construct("kurang Rp" . number_format($kurang, 0, ',', '.'));
    }
    public function getKurang(): int {
        return $this->kurang;
    }
}

function tarik(int $saldo, int $jumlah): int {
    if ($jumlah > $saldo) {
        throw new SaldoTidakCukup($jumlah - $saldo);
    }
    return $saldo - $jumlah;
}

try {
    echo tarik(50000, 20000), "\n";
    echo tarik(10000, 125000), "\n";
} catch (SaldoTidakCukup $e) {
    echo "Gagal: ", $e->getMessage(), " (", $e->getKurang(), ")\n";
} finally {
    echo "transaksi selesai\n";
}

$nilai = ["Ani" => 85, "Budi" => 72, "Cici" => 91, "Dodi" => 64];
arsort($nilai);
print_r($nilai);
$lulus = array_filter($nilai, fn($n) => $n >= 70);
echo implode(", ", array_keys($lulus)), " | rata-rata: ", round(array_sum($nilai) / count($nilai), 1), "\n";
$huruf = array_map(fn($n) => match (true) {
    $n >= 90 => "A",
    $n >= 80 => "B",
    $n >= 70 => "C",
    default => "D",
}, $nilai);
var_dump($huruf);
echo json_encode(["nama" => "Ani", "nilai" => [85, 90], "lulus" => true, "url" => "a/b"]), "\n";
echo json_encode(["x" => 1.5, "y" => null], JSON_PRETTY_PRINT), "\n";

function tambahSatu(array &$arr): void {
    foreach ($arr as &$v) {
        $v++;
    }
}
$angka = [1, 2, 3];
tambahSatu($angka);
echo implode(",", $angka), "\n";

function hitung(): int {
    static $n = 0;
    return ++$n;
}
hitung();
hitung();
echo "dipanggil ", hitung(), " kali\n";

$pengali = 3;
$kali = function ($x) use ($pengali) {
    return $x * $pengali;
};
$tambah = fn($x) => $x + $pengali;
echo $kali(5), " ", $tambah(5), "\n";

[$a, [$b, $c]] = [1, [2, 3]];
['x' => $x, 'y' => $y] = ['x' => 10, 'y' => 20];
echo "$a $b $c $x $y\n";
$config = ['debug' => false];
$config['level'] ??= 'info';
echo $config['level'], " ", $config['missing'] ?? 'default', " ", isset($config['debug']) ? 'ada' : 'tidak', "\n";

$matriks = [];
for ($i = 1; $i <= 3; $i++) {
    for ($j = 1; $j <= 3; $j++) {
        $matriks[$i][$j] = $i * $j;
    }
}
echo $matriks[2][3], " ", count($matriks), " ", count($matriks, COUNT_RECURSIVE), "\n";
$kata = "Belajar PHP itu Seru";
printf("%-10s|%5d|%05.2f|%x|%s\n", strtoupper(substr($kata, 0, 7)), strlen($kata), 3.14159, 255, ucwords(strtolower($kata)));
echo str_pad("7", 3, "0", STR_PAD_LEFT), " ", str_repeat("=", 5), " ", strrev("abc"), " ", ucfirst("halo"), " ", wordCount($kata), "\n";

function wordCount(string $s): int {
    return count(explode(" ", $s));
}

enum Status: string {
    case Aktif = 'aktif';
    case Nonaktif = 'nonaktif';

    public function label(): string {
        return match ($this) {
            Status::Aktif => 'Sedang aktif',
            Status::Nonaktif => 'Tidak aktif',
        };
    }
}
echo Status::Aktif->value, " ", Status::Nonaktif->label(), " ", count(Status::cases()), "\n";

$nama = "Dunia";
$teks = <<<EOT
Halo, $nama!
Jumlah: {$nilai['Ani']}
EOT;
echo $teks, "\n";
echo intdiv(17, 5), " ", 17 % 5, " ", 2 ** 10, " ", 7 / 2, " ", "10" + 5, " ", 0.1 + 0.2 == 0.3 ? "sama" : "beda", "\n";
var_dump(1 <=> 2, "abc" == 0, null ?? "kosong", (int) "42abc", (string) 3.0);
