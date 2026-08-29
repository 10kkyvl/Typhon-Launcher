export interface TokenNameCase {
  title: string;
  name: string;
  valid: boolean;
}

export const allowedTokenNamesFixture = ['--bg', '--surface', '--text', '--accent', '--danger'];

export const tokenNameCases: TokenNameCase[] = [
  { title: 'разрешённый токен', name: '--bg', valid: true },
  { title: 'другой разрешённый токен', name: '--accent', valid: true },
  { title: 'токен вне списка', name: '--made-up', valid: false },
  { title: 'зарезервированный --ui-scale', name: '--ui-scale', valid: false },
  { title: 'пустое имя', name: '', valid: false },
];

export interface TokenValueCase {
  title: string;
  value: string;
  valid: boolean;
}

export const tokenValueCases: TokenValueCase[] = [
  { title: 'hex-цвет', value: '#10161d', valid: true },
  { title: 'rgba', value: 'rgba(104, 117, 232, 0.14)', valid: true },
  { title: 'размер в rem', value: '1.2rem', valid: true },
  { title: 'составное значение', value: '0 0.8rem 2.8rem rgba(0, 0, 0, 0.45)', valid: true },
  { title: 'пустое значение', value: '', valid: false },
  { title: 'слишком длинное значение', value: '#'.repeat(121), valid: false },
  { title: 'точка с запятой', value: '#fff; color: red', valid: false },
  { title: 'url()', value: 'url(javascript:alert(1))', valid: false },
  { title: 'expression()', value: 'expression(alert(1))', valid: false },
  { title: 'комментарий', value: '/* x */', valid: false },
  { title: 'обратный слэш', value: 'a\\b', valid: false },
  { title: 'собачка', value: '@media', valid: false },
  { title: 'фигурные скобки', value: '{color:red}', valid: false },
];

export interface CssCase {
  title: string;
  css: string;
  valid: boolean;
}

export const cssCases: CssCase[] = [
  { title: 'пустой css', css: '', valid: true },
  { title: 'простое правило', css: '.card { color: red; }', valid: true },
  { title: 'несколько правил', css: '.a { color: red; }\n.b { background: blue; }', valid: true },
  { title: 'import', css: '@import url(evil.css);', valid: false },
  { title: 'url()', css: 'a { background: url(image.png); }', valid: false },
  { title: 'expression()', css: '.a { width: expression(alert(1)); }', valid: false },
  { title: 'javascript:', css: 'a { background: url(javascript:alert(1)); }', valid: false },
  { title: 'behavior', css: 'a { behavior: url(x.htc); }', valid: false },
  { title: 'moz-binding', css: 'a { -moz-binding: url(x.xml); }', valid: false },
  { title: 'закрывающий style', css: '</style><script>alert(1)</script>', valid: false },
  { title: 'незакрытая открывающая скобка', css: '.card { color: red;', valid: false },
  { title: 'лишняя закрывающая скобка', css: '.card { color: red; }}', valid: false },
  { title: 'слишком длинный css', css: 'a{color:red}'.repeat(3000), valid: false },
  { title: '!important разрешён: без него тему не написать', css: '.card { color: red !important; }', valid: true },
  { title: 'токен темы через css разрешён', css: ':root { --accent: red; }', valid: true },
  { title: 'подмена --ui-scale через css', css: ':root { --ui-scale: 4 !important; }', valid: false },
  { title: 'упоминание --ui-scale без !important', css: ':root { --ui-scale: 1.1; }', valid: false },
];
