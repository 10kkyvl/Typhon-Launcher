import type { Dlc, GameAchievements } from './types';

export const achievements: Record<string, GameAchievements> = {
  'cyberpunk-2077': {
    earned: 57,
    total: 57,
    recent: [
      { name: 'Душа Найт-Сити', description: 'Получите все достижения.', date: '28 апр. 2024 г., 17:36' },
      { name: 'Легенда Афтерлайфа', description: 'Завершите все заказы и побочные задания в Найт-Сити.', date: '28 апр. 2024 г., 16:02' },
      { name: 'Прирождённый гонщик', description: 'Победите во всех гонках Клэр.', date: '26 апр. 2024 г., 21:14' },
    ],
  },
  'elden-ring': {
    earned: 31,
    total: 42,
    recent: [
      { name: 'Повелитель Элдена', description: 'Достигните концовки «Повелитель Элдена».', date: '14 июн. 2024 г., 23:47' },
      { name: 'Легендарное вооружение', description: 'Соберите все легендарные виды оружия.', date: '10 июн. 2024 г., 19:05' },
    ],
  },
  'witcher-3': {
    earned: 78,
    total: 78,
    recent: [
      { name: 'Ходячая энциклопедия', description: 'Узнайте все слабые места 20 типов противников.', date: '3 фев. 2024 г., 20:11' },
    ],
  },
  'baldurs-gate-3': {
    earned: 34,
    total: 54,
    recent: [
      { name: 'Абсолютное решение', description: 'Завершите игру.', date: '17 мая 2024 г., 22:30' },
      { name: 'Критический провал', description: 'Выбросьте 1 на кубике в решающий момент.', date: '12 мая 2024 г., 21:47' },
    ],
  },
  'hogwarts-legacy': {
    earned: 18,
    total: 45,
    recent: [
      { name: 'Первое испытание', description: 'Пройдите первое испытание Мерлина.', date: '2 июн. 2024 г., 18:20' },
    ],
  },
  'forza-horizon-5': {
    earned: 42,
    total: 110,
    recent: [
      { name: 'Добро пожаловать в Мексику', description: 'Завершите вступление фестиваля Horizon.', date: '20 мая 2024 г., 14:55' },
    ],
  },
};

export const dlcs: Dlc[] = [
  { id: 'dlc-pl', gameId: 'cyberpunk-2077', name: 'Phantom Liberty', kind: 'Дополнение', installed: true },
  { id: 'dlc-bonus', gameId: 'cyberpunk-2077', name: 'Cyberpunk 2077: Бонусный контент', kind: 'Дополнение', installed: true },
  { id: 'dlc-wall', gameId: 'cyberpunk-2077', name: 'Cyberpunk 2077: Набор обоев', kind: 'Дополнение', installed: true },
  { id: 'dlc-er-dlc', gameId: 'elden-ring', name: 'Shadow of the Erdtree', kind: 'Дополнение', installed: false },
  { id: 'dlc-w3-hos', gameId: 'witcher-3', name: 'Каменные сердца', kind: 'Дополнение', installed: true },
  { id: 'dlc-w3-baw', gameId: 'witcher-3', name: 'Кровь и вино', kind: 'Дополнение', installed: true },
];
