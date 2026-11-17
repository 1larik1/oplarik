// 1.1.2: light themes and theme groups; the overlay takes the theme's colors.
const K = ['th.dark', 'th.lightGroup', 'th.light', 'th.sky', 'th.lagoon', 'th.mint', 'th.sand', 'th.rose', 'th.paper', 'th.sub'];
const V = {
  en: ['Dark', 'Light', 'Lavender', 'Sky', 'Lagoon', 'Mint', 'Sand', 'Rose', 'Paper', 'Interface colors. Change instantly; the overlay takes the same colors.'],
  ru: ['Тёмные', 'Светлые', 'Лаванда', 'Небо', 'Лагуна', 'Мята', 'Песок', 'Роза', 'Бумага', 'Цвета интерфейса. Меняются сразу, оверлей берёт те же цвета.'],
  uk: ['Темні', 'Світлі', 'Лаванда', 'Небо', 'Лагуна', 'М’ята', 'Пісок', 'Троянда', 'Папір', 'Кольори інтерфейсу. Змінюються одразу, оверлей бере ті самі кольори.'],
  pl: ['Ciemne', 'Jasne', 'Lawenda', 'Niebo', 'Laguna', 'Mięta', 'Piasek', 'Róża', 'Papier', 'Kolory interfejsu. Zmieniają się od razu, nakładka przejmuje te same kolory.'],
  es: ['Oscuros', 'Claros', 'Lavanda', 'Cielo', 'Laguna', 'Menta', 'Arena', 'Rosa', 'Papel', 'Colores de la interfaz. Cambian al instante; la superposición usa los mismos colores.'],
  de: ['Dunkel', 'Hell', 'Lavendel', 'Himmel', 'Lagune', 'Minze', 'Sand', 'Rose', 'Papier', 'Farben der Oberfläche. Ändern sich sofort; das Overlay übernimmt dieselben Farben.'],
  fr: ['Sombres', 'Clairs', 'Lavande', 'Ciel', 'Lagon', 'Menthe', 'Sable', 'Rose', 'Papier', 'Couleurs de l’interface. Changent immédiatement ; l’overlay prend les mêmes couleurs.'],
  ja: ['ダーク', 'ライト', 'ラベンダー', 'スカイ', 'ラグーン', 'ミント', 'サンド', 'ローズ', 'ペーパー', 'インターフェイスの配色です。すぐに切り替わり、オーバーレイも同じ配色になります。'],
  cs: ['Tmavé', 'Světlé', 'Levandule', 'Nebe', 'Laguna', 'Máta', 'Písek', 'Růže', 'Papír', 'Barvy rozhraní. Mění se okamžitě; overlay přebírá stejné barvy.'],
};
export const S112 = Object.fromEntries(Object.entries(V).map(([l, v]) => [l, Object.fromEntries(K.map((k, i) => [k, v[i]]))]));
