import bisect

values = [4, 8, 2, 9, 5, 7]
if 9 in values:
    print("ada")

def posisi():
    return values.index(5)
print(posisi())

urut = [1, 3, 5, 7, 9, 11, 13, 15]
def sisip():
    return bisect.bisect_left(urut, 11)
print(sisip())
