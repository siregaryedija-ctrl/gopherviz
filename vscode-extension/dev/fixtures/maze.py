from collections import deque

grid = [
    "S..#....",
    ".#.#.##.",
    ".#...#..",
    ".####.#.",
    "......#G",
]

def bfs():
    rows, cols = len(grid), len(grid[0])
    start = (0, 0)
    visited = [[False] * cols for _ in range(rows)]
    visited[0][0] = True
    q = deque([(0, 0, 0)])
    while q:
        r, c, d = q.popleft()
        if grid[r][c] == "G":
            return d
        for dr, dc in [(1, 0), (-1, 0), (0, 1), (0, -1)]:
            nr, nc = r + dr, c + dc
            if 0 <= nr < rows and 0 <= nc < cols and not visited[nr][nc] and grid[nr][nc] != "#":
                visited[nr][nc] = True
                q.append((nr, nc, d + 1))
    return -1

print(bfs())
