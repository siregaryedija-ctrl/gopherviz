maze = [
    "S.#.....",
    ".##.###.",
    "...#...#",
    "#.##.#..",
    "...#.#.#",
    ".#...#.G",
]
visited = set()


def dfs(r, c):
    if r < 0 or c < 0 or r >= len(maze) or c >= len(maze[0]):
        return False
    if maze[r][c] == "#" or (r, c) in visited:
        return False
    visited.add((r, c))
    if maze[r][c] == "G":
        return True
    for dr, dc in [(0, 1), (1, 0), (0, -1), (-1, 0)]:
        if dfs(r + dr, c + dc):
            return True
    return False


print(dfs(0, 0))
