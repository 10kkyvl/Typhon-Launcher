import type { CompatInfo } from '../services/sources';

// Порог, выше которого игра считается работающей. Сервер не отдаёт строк, по
// которым нечего считать, поэтому здесь остаётся только вопрос «у большинства
// или нет».
const worksAbove = 0.5;

export interface CompatBadge {
  works: boolean;
  works_count: number;
  total: number;
}

// compatBadge возвращает null, когда показывать нечего. Отсутствие строки — не
// «ноль процентов»: сервер отдаёт цифру только выше порога наблюдений, и
// молчание тут честнее любого числа.
export function compatBadge(compat: CompatInfo | undefined): CompatBadge | null {
  if (!compat || compat.total <= 0) return null;
  if (compat.works < 0 || compat.works > compat.total) return null;
  return {
    works: compat.works / compat.total >= worksAbove,
    works_count: compat.works,
    total: compat.total,
  };
}
