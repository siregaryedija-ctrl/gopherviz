class Orang:
    def __init__(self, nama):
        self.nama = nama

class Siswa(Orang):
    def __init__(self, nama, nilai):
        super().__init__(nama)
        self.nilai = nilai
        self.lulus = False

    def tambah(self, n):
        self.nilai += n
        if self.nilai >= 75:
            self.lulus = True
        return self.nilai

ani = Siswa("Ani", 70)
budi = Siswa("Budi", 60)
ani.tambah(10)
budi.tambah(5)
budi.nilai = 99
print(ani.nilai, budi.nilai)
