def total(values):
    s = 0
    for i in range(len(values) + 1):
        s += values[i]
    return s


print(total([4, 7, 9, 2]))
