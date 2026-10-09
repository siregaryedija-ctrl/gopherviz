const maze = [
  "S.#..",
  ".##.#",
  "....G",
];
function bfs() {
  const rows = maze.length, cols = maze[0].length;
  const seen = maze.map(r => Array(cols).fill(false));
  const queue = [[0, 0]];
  seen[0][0] = true;
  while (queue.length > 0) {
    const [r, c] = queue.shift();
    if (maze[r][c] === "G") return true;
    for (const [dr, dc] of [[1, 0], [-1, 0], [0, 1], [0, -1]]) {
      const nr = r + dr, nc = c + dc;
      if (nr >= 0 && nr < rows && nc >= 0 && nc < cols && !seen[nr][nc] && maze[nr][nc] !== "#") {
        seen[nr][nc] = true;
        queue.push([nr, nc]);
      }
    }
  }
  return false;
}
console.log(bfs());
