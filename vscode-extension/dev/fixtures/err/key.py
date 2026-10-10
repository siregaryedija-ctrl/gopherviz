def harga(menu, nama):
    return menu[nama]

menu = {"kopi": 8000, "teh": 5000, "roti": 12000}
print(harga(menu, "teh"))
print(harga(menu, "susu"))
