grid = [
    [1, 1, 0, 0],
    [1, 0, 0, 1],
    [0, 0, 1, 1],
]

def isi(r, c):
    if r < 0 or c < 0 or r >= len(grid) or c >= len(grid[0]) or grid[r][c] != 1:
        return
    grid[r][c] = 2
    isi(r + 1, c)
    isi(r - 1, c)
    isi(r, c + 1)
    isi(r, c - 1)

pulau = 0
for r in range(len(grid)):
    for c in range(len(grid[0])):
        if grid[r][c] == 1:
            pulau += 1
            isi(r, c)
print(pulau)
