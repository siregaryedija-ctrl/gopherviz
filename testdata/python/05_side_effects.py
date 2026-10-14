counter = 0


def tick():
    global counter
    counter += 1
    print("tick", counter)
    return counter


def main():
    arr = [0, 0, 0, 0, 0]
    st = [1, 2, 3, 4]
    while st and st.pop() > 2:
        print("popped")
    if tick() + 0 > 5:
        print("big")
    total = 0
    total += tick()
    n = 0
    while (n := n + 1) < 3:
        print("walrus", n)
    q = [5, 6, 7]
    while q.pop(0) < 7:
        print("shift", len(q))
    x = tick() if counter > 1 else 0
    print(arr[0], len(st), counter, total, n, len(q), x)


main()
