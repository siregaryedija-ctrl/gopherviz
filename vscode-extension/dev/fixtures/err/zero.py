def rata(nilai):
    total = 0
    for x in nilai:
        total += x
    return total / len(nilai)

print(rata([3, 5]))
print(rata([]))
