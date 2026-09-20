/* Microsoft Clarity，由实例自己决定开不开。
 *
 * 不写在 index.html 里，有两个原因。一是这是个自托管的程序：把某一家的项目 id
 * 印进发布出去的壳，等于每个自建的人都在替别人收集访客数据，而他们不会知道。
 * 二是 CSP —— script-src 是 'self' 加内联哈希，外部脚本要专门放行，而那个放行
 * 只在实例真的配了 id 的时候才给（见 httpx.SecurityHeaders）。没配就不放行，
 * 这里也不会去加载。
 *
 * 加载失败不做任何处理：统计挂了不该影响聊天。 */

let started = '';

export function startAnalytics(projectId: string | undefined | null): void {
  const id = String(projectId || '').trim();
  // 只认字母数字：这个值会拼进脚本地址，而它来自服务器的设置项。
  if (!id || !/^[a-z0-9]+$/i.test(id) || started === id) return;
  if (typeof document === 'undefined') return;
  started = id;

  const clarity = (window as unknown as Record<string, unknown>).clarity as
    | (((...args: unknown[]) => void) & { q?: unknown[] })
    | undefined;
  if (!clarity) {
    const queue: unknown[] = [];
    const shim = (...args: unknown[]) => { queue.push(args); };
    shim.q = queue;
    (window as unknown as Record<string, unknown>).clarity = shim;
  }

  const tag = document.createElement('script');
  tag.async = true;
  tag.src = `https://www.clarity.ms/tag/${id}`;
  document.head.appendChild(tag);
}
