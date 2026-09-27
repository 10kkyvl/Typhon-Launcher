export function nextGenre(current: string, selected: string): string {
  return selected !== '' && selected === current ? '' : selected;
}
