export function shouldRenderFrame(inView: boolean, pageVisible: boolean): boolean {
  return inView && pageVisible;
}
