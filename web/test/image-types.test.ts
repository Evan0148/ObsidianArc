import { describe, it, expect } from 'vitest';
import { isSupportedImageType } from '../src/chat/image';

// What prepareImage can decode. Everything except GIF is re-encoded to JPEG
// before it is sent, so the question this answers is what the browser can
// read — not what a provider accepts.
describe('the image types that can be prepared', () => {
  // AVIF is offered by the wallpaper picker's accept list, by the image
  // composer's `image/*`, and stored by the server as image/avif. It was
  // missing here, so choosing the file the dialog had just offered decoded
  // successfully and then failed the allowlist with a format error.
  it('covers every type the pickers offer', () => {
    for (const type of ['image/png', 'image/jpeg', 'image/webp', 'image/avif', 'image/gif']) {
      expect(isSupportedImageType(type), type).toBe(true);
    }
  });

  it('still refuses a type that is not on the list', () => {
    for (const type of ['image/tiff', 'image/bmp', 'application/pdf', 'text/plain', '']) {
      expect(isSupportedImageType(type), type).toBe(false);
    }
  });
});
