class Folder:
    def __init__(self, name, children=None):
        self.name = name
        self.children = children or []

def tampil(f, depth):
    print("  " * depth + f.name)
    for c in f.children:
        tampil(c, depth + 1)

tampil(Folder("root", [Folder("src", [Folder("main")]), Folder("docs")]), 0)
