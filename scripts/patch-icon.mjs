import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { rcedit } from '../ui/node_modules/rcedit/lib/index.js';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const exe = path.join(root, 'build', 'bin', 'Ignore.exe');
const icon = path.join(root, 'build', 'windows', 'icon.ico');

await rcedit(exe, { icon });

console.log(`Patched icon: ${exe}`);
