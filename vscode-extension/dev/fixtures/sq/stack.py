def cek_kurung(teks):
    tumpukan = []
    pasangan = {")": "(", "]": "[", "}": "{"}
    for ch in teks:
        if ch in "([{":
            tumpukan.append(ch)
        elif ch in ")]}":
            if not tumpukan or tumpukan.pop() != pasangan[ch]:
                return False
    return len(tumpukan) == 0

print(cek_kurung("{[()()]}"))
