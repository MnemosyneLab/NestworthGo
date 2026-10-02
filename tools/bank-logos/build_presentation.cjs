/** Regenerate the reviewed display catalog with Chromium path bounds.
 * NODE_PATH=/path/to/node_modules node tools/bank-logos/build_presentation.cjs
 * Requires playwright and Chromium (CHROMIUM_PATH may override the executable).
 * Original assets, stable keys and backend allowlist remain untouched.
 */
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require('playwright');
const root = path.resolve(__dirname, '../..');
(async () => {
  const catalog = JSON.parse(fs.readFileSync(path.join(root, 'frontend/src/lib/bankLogos.json')));
  const reviewed = JSON.parse(fs.readFileSync(path.join(__dirname, 'presentation.json')));
  const byKey = new Map(catalog.map(logo => [logo.key, logo]));
  const covered = reviewed.flatMap(entry => [entry.key, ...entry.legacyKeys]);
  if (covered.length !== catalog.length || new Set(covered).size !== catalog.length || covered.some(key => !byKey.has(key))) throw Error('Every stable ID must have exactly one presentation');
  const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || '/usr/bin/chromium', args: ['--no-sandbox'] });
  try {
    const page = await browser.newPage();
    const output = path.join(root, 'frontend/public/bank-logos/display');
    fs.mkdirSync(output, { recursive: true });
    const display = [];
    for (const entry of reviewed) {
      const logo = byKey.get(entry.key);
      const raw = fs.readFileSync(path.join(root, 'frontend/public/bank-logos', logo.file), 'utf8');
      const viewBox = await page.evaluate(raw => {
        document.body.innerHTML = raw; // Only already-screened, vendored SVGs; never application input.
        const svg = document.querySelector('svg');
        const vb = svg.viewBox.baseVal;
        const boxes = [...svg.children].map(node => ({node, box: node.getBBox()})).filter(({node, box}, i) => {
          // Ignore only the full-canvas white backplate when measuring. Keep it
          // in the output so white counters and brand colors remain unchanged.
          const white = /^#(?:fff|ffffff)$/i.test(node.getAttribute('fill') || '');
          return !(i === 0 && white && Math.abs(box.x-vb.x)<vb.width*.01 && Math.abs(box.y-vb.y)<vb.height*.01 && Math.abs(box.width-vb.width)<vb.width*.01 && Math.abs(box.height-vb.height)<vb.height*.01);
        }).map(x => x.box).filter(b => b.width > 0 && b.height > 0);
        if (!boxes.length) throw Error('Empty artwork');
        const x = Math.min(...boxes.map(b => b.x)), y = Math.min(...boxes.map(b => b.y));
        const w = Math.max(...boxes.map(b => b.x+b.width))-x, h = Math.max(...boxes.map(b => b.y+b.height))-y;
        const pad = Math.max(w,h)*.035;
        return [x-pad,y-pad,w+pad*2,h+pad*2].map(n => Number(n.toFixed(4))).join(' ');
      }, raw);
      fs.writeFileSync(path.join(output, logo.file), raw.replace(/viewBox="[^"]+"/, `viewBox="${viewBox}"`));
      const aliases = [...new Set([entry.key, ...entry.legacyKeys].flatMap(key => {const item=byKey.get(key);return [item.name,...item.aliases]}))];
      const name = logo.file.startsWith('other-') ? logo.name.replace(/logo_?$/, '').replace(/^!/, '') : logo.aliases[1];
      display.push({...entry, name, aliases, file: `display/${logo.file}`});
    }
    fs.writeFileSync(path.join(root, 'frontend/src/lib/bankLogoPresentation.json'), JSON.stringify(display,null,2)+'\n');
    console.log(`Built ${display.length} reviewed presentations for ${covered.length} stable IDs`);
  } finally { await browser.close(); }
})().catch(error => {console.error(error);process.exitCode=1});
