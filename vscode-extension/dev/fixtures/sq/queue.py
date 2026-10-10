from collections import deque

def layani(pelanggan):
    antrian = deque()
    for p in pelanggan:
        antrian.append(p)
    while antrian:
        orang = antrian.popleft()
        print("melayani", orang)

layani(["Ani", "Budi", "Cici", "Dodi"])
