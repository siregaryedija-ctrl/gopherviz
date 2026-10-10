def hitung_mundur(n):
    print(n)
    if n == 0:
        return
    hitung_mundur(n + 1)

hitung_mundur(3)
