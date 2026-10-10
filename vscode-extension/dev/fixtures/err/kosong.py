def layani(antrian):
    while True:
        orang = antrian.pop(0)
        print("melayani", orang)

layani(["Ani", "Budi"])
