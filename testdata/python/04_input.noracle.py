# Input user tidak diketahui saat simulasi: GopherViz mengisinya dengan angka acak.
def kategori_usia(usia):
    assert usia >= 0, "Usia tidak boleh negatif!"
    if 0 <= usia <= 5:
        return "Bayi (Toddler)"
    elif 5 <= usia <= 6:
        return "Prasekolah"
    elif 6 <= usia <= 10:
        return "Anak-Anak"
    elif 10 <= usia <= 19:
        return "Transisi ke remaja"
    else:
        return "Dewasa"


try:
    input_user = int(input("Masukkan umur kamu sekarang: "))
    kategori = kategori_usia(input_user)
    print(f"Input Usia ({input_user} tahun), \nMasuk ke dalam kategori: {kategori}")
except AssertionError as e:
    print(f"Error Assertion: {e}")
except ValueError:
    print("Error Input: Harap masukkan angka yang valid!")

total = 0
for i in range(3):
    total += int(input())
print("total", total)
