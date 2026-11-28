/**
 * 仓库根路径（scripts/ 下的脚本共用）。
 *
 * 不能再用 `new URL('..', import.meta.url).pathname`：URL.pathname 返回百分号
 * 编码路径且 Windows 盘符前多一个 `/`，检出路径含空格/中文时（D:\My Projects、
 * C:\Users\张三）ROOT 会变成 `D:\My%20Projects\...`，脚本误报 webroot 缺失。
 * fileURLToPath 负责解码与平台路径转换。
 */
import { fileURLToPath } from 'node:url';
import { resolve } from 'node:path';

const ROOT = resolve(fileURLToPath(new URL('../..', import.meta.url)));

export { ROOT };
