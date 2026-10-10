def cari(daftar, x):
    for i in range(len(daftar)):
        if daftar[i] == x:
            return i

def sapa(nama):
    print("halo", nama.upper())

posisi = cari([3, 8, 1], 5)
sapa(None)
