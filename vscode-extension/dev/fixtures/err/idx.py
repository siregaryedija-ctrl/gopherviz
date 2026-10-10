def total(nilai):
    jumlah = 0
    for i in range(len(nilai) + 1):
        jumlah += nilai[i]
    return jumlah

print(total([4, 7, 9, 2]))
