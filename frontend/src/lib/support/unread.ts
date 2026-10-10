export function unreadGrew(previous: number | null, next: number): boolean {
  return previous !== null && next > previous;
}
