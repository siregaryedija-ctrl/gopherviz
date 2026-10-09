import utils
from utils import diskon

def kasir():
    harga = [100, 250, 80]
    print(diskon(200, 50))
    bayar = utils.total(harga)
    print("bayar", bayar)
    return bayar

kasir()
