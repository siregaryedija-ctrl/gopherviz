from collections import Counter, defaultdict, deque
import math


class Vector:
    count = 0

    def __init__(self, x, y):
        self.x = x
        self.y = y
        Vector.count += 1

    def __add__(self, other):
        return Vector(self.x + other.x, self.y + other.y)

    def __eq__(self, other):
        return self.x == other.x and self.y == other.y

    def __lt__(self, other):
        return self.length < other.length

    def __repr__(self):
        return f"Vector({self.x}, {self.y})"

    @property
    def length(self):
        return math.hypot(self.x, self.y)

    @classmethod
    def zero(cls):
        return cls(0, 0)

    @staticmethod
    def dot(a, b):
        return a.x * b.x + a.y * b.y


v = Vector(3, 4) + Vector(1, 1)
print(v, v.length, Vector.zero(), Vector.dot(v, Vector(1, 0)), Vector.count)
print(sorted([Vector(5, 5), Vector(1, 0), Vector(2, 2)]), Vector(1, 2) == Vector(1, 2))


class Rekening:
    def __init__(self, pemilik, saldo=0):
        self.pemilik = pemilik
        self._saldo = saldo

    @property
    def saldo(self):
        return self._saldo

    @saldo.setter
    def saldo(self, nilai):
        if nilai < 0:
            raise ValueError("saldo tidak boleh negatif")
        self._saldo = nilai

    def __str__(self):
        return f"{self.pemilik}: Rp{self._saldo:,}"


class SaldoKurang(Exception):
    pass


r = Rekening("Ani", 50000)
r.saldo = 125000
print(r)
try:
    r.saldo = -1
except ValueError as e:
    print("gagal:", e)
try:
    raise SaldoKurang("butuh 1000 lagi")
except SaldoKurang as e:
    print(type(e).__name__, e.args, str(e))
else:
    print("tidak terjadi")
finally:
    print("cek selesai")


def counter():
    n = 0

    def inc(step=1):
        nonlocal n
        n += step
        return n
    return inc


c = counter()
c()
c(5)
print(c())


def tambah(item, daftar=[]):
    daftar.append(item)
    return daftar


print(tambah(1), tambah(2))


def info(*args, sep="-", **kwargs):
    return sep.join(str(a) for a in args) + " " + str(sorted(kwargs.items()))


print(info(1, 2, 3, sep="+", b=2, a=1))

nums = [5, 3, 8, 1, 9, 2]
print(nums[::2], nums[-2:], nums[::-1][:3], [x ** 2 for x in nums if x % 2])
print({x: x * x for x in range(4)}, {n % 3 for n in nums}, sum(x for x in nums))
first, *middle, last = nums
print(first, middle, last)
if (n := len(nums)) > 5:
    print("panjang", n)
print(1 < 2 < 3, 3 > 2 > 2, 7 // -2, -7 % 3, divmod(17, 5), round(2.5), round(3.5), round(2.675, 2))
print(2 ** 100, math.factorial(25), 10 / 4, 10 // 4, abs(-7.5), int("1010", 2), hex(255))
words = "the quick brown fox jumps over the lazy dog the end".split()
cnt = Counter(words)
print(cnt.most_common(2), cnt["the"], cnt["zebra"])
groups = defaultdict(list)
for w in words:
    groups[len(w)].append(w)
print(dict(groups))
q = deque([1, 2, 3])
q.append(4)
q.appendleft(0)
print(q.popleft(), q.pop(), q)
print(f"{math.pi:.3f} {12345.6789:,.2f} {'kiri':<8}|{'kanan':>8}|{'tengah':^10}| {0.875:.1%} {42:05d}")
print("%s punya %d apel seharga %.2f" % ("Budi", 3, 4500.5), "{:>5}|{:<5}|".format("a", "b"))
print(" Halo Dunia ".strip().lower().replace("dunia", "python").title(), "abc".upper().center(9, "*"))
print(list(zip("abc", range(3))), list(enumerate("xy", start=1)), list(map(str.upper, ["a", "b"])))
print(any(x > 8 for x in nums), all(x > 0 for x in nums), min(nums), max(nums, key=lambda x: -x))
for i in range(2, 10):
    for j in range(2, i):
        if i % j == 0:
            break
    else:
        print(i, "prima")


def area(shape):
    match shape:
        case ("circle", r):
            return round(math.pi * r * r, 2)
        case ("rect", w, h) if w == h:
            return f"persegi {w * h}"
        case ("rect", w, h):
            return w * h
        case _:
            return None


print(area(("circle", 1)), area(("rect", 2, 2)), area(("rect", 2, 3)), area("x"))
data = {"b": 2, "a": 1}
data.update(c=3)
for k, val in sorted(data.items()):
    print(k, val, end="; ")
print()
print({3, 1, 2}, set(range(10, 0, -2)), {100, 5, 33, 8}, len({1, 1, 2}))

# operator or (perbandingan lalu or, comprehension, ternary)
kosong = 0
print(kosong or "default", kosong < 1 or kosong > 9, [v for v in [] or [1, 2, 3] if v == 1 or v == 3], "ya" if kosong or 1 else "tidak")
