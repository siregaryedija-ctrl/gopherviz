# Dasar Python
def fib(n):
    if n < 2:
        return n
    return fib(n - 1) + fib(n - 2)


nums = [5, 3, 8, 1]
total = 0
for i in range(len(nums)):
    total += nums[i]
print("total", total)

for x in nums:
    if x > 4:
        print(x, "besar")
    elif x > 2:
        print(x, "sedang")
    else:
        print(x, "kecil")

k = 3
while k > 0:
    k -= 1
else:
    print("while selesai")

stok = {"apel": 3, "jeruk": 0}
for nama, jumlah in stok.items():
    print(f"{nama}: {jumlah:>3} buah")

kuadrat = [n * n for n in nums if n % 2 == 1]
pasangan = {n: n * 2 for n in range(3)}
a, b = 1, 2
a, b = b, a
first, *rest = nums
print(kuadrat, pasangan, a, b, first, rest, nums[1:3], nums[::-1])
print(fib(6), 1 < a < 3, "x" if a else "y", lambda_ := (lambda v: v + 1)(2))


class Hewan:
    jumlah = 0

    def __init__(self, nama: str, suara: str = "..."):
        self.nama = nama
        self.suara = suara
        Hewan.jumlah += 1

    def bicara(self) -> str:
        return f"{self.nama} berkata {self.suara!r}"


class Kucing(Hewan):
    def __init__(self, nama):
        super().__init__(nama, "meong")


try:
    h = Kucing("Tom")
    print(h.bicara())
    hasil = 10 / 0
except ZeroDivisionError as e:
    print("error:", e)
finally:
    print("selesai")

match k:
    case 0:
        print("nol")
    case 1 | 2:
        print("kecil")
    case _:
        print("lain")
