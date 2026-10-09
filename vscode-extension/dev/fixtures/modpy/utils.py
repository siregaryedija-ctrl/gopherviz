PAJAK = 10

def diskon(harga, persen):
    potong = harga * persen // 100
    return harga - potong

def total(daftar):
    jumlah = 0
    for h in daftar:
        jumlah += diskon(h, PAJAK)
    return jumlah
