import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createApp, h, nextTick, shallowRef, type App } from 'vue';
import { createRouter, createWebHistory } from 'vue-router';
import * as imagesApi from '@/api/images';
import * as chatImage from '@/chat/image';
import { models } from '@/chat/useModels';
import { providePanelHost } from '@/composables/usePanelHost';
import { changeLanguage, t } from '@/composables/useI18n';
import ImageLabPanel from '@/views/ImageLabPanel.vue';

let app: App | undefined;
let host: HTMLElement;
let panels: HTMLElement;

const router = createRouter({
  history: createWebHistory(),
  routes: [{ path: '/image-lab', component: ImageLabPanel }],
});

beforeEach(async () => {
  await changeLanguage('en');
  host = document.createElement('div');
  panels = document.createElement('div');
  document.body.append(host, panels);

  models.value = [
    {
      id: 'mock-painter',
      display_name: 'Mock Painter',
      supports_image_gen: true,
      usable: true,
    } as any,
  ];

  vi.spyOn(imagesApi, 'listImageGenerations').mockResolvedValue({ generations: [] });
  vi.spyOn(chatImage, 'prepareImage').mockImplementation(async (file: File) => {
    if (!file.type.startsWith('image/')) {
      throw new chatImage.ImageError('unsupported', 'This file is not an image.');
    }
    return {
      mime: file.type,
      data: `base64-${file.name}`,
      width: 1024,
      height: 1024,
      bytes: file.size,
      previewURL: `blob:mock/${file.name}`,
    };
  });
});

afterEach(() => {
  app?.unmount();
  app = undefined;
  host.remove();
  panels.remove();
  document.body.textContent = '';
  vi.restoreAllMocks();
});

async function settle(): Promise<void> {
  for (let i = 0; i < 3; i++) {
    await new Promise((resolve) => setTimeout(resolve, 0));
    await nextTick();
  }
}

async function mountImageLab(): Promise<void> {
  await router.push('/image-lab');
  app = createApp({
    setup() {
      providePanelHost(shallowRef(panels));
      return () => h(ImageLabPanel);
    },
  });
  app.use(router);
  app.mount(host);
  await settle();
}

function createFile(name: string, type: string): File {
  return new File(['mock content'], name, { type });
}

function createDragEvent(type: string, files: File[] = []): DragEvent {
  const event = new Event(type, { bubbles: true, cancelable: true }) as any;
  event.dataTransfer = {
    types: files.length ? ['Files'] : [],
    files,
    dropEffect: 'none',
  };
  return event;
}

function createPasteEvent(files: File[]): ClipboardEvent {
  const event = new Event('paste', { bubbles: true, cancelable: true }) as any;
  event.clipboardData = {
    items: files.map((file) => ({
      kind: 'file',
      type: file.type,
      getAsFile: () => file,
    })),
  };
  return event;
}

describe('Image Lab reference image drag and drop', () => {
  it('renders the dedicated dropzone when no reference images are attached', async () => {
    await mountImageLab();

    const dropzone = panels.querySelector('.oa-reference-dropzone');
    expect(dropzone).not.toBeNull();
    expect(dropzone?.textContent).toContain(t('referenceImageDrop'));
    expect(dropzone?.textContent).toContain(t('referenceImageDropHint'));
    expect(panels.querySelectorAll('.oa-reference').length).toBe(0);
  });

  it('activates drag state with drop hint when dragging images over the form and handles depth tracking', async () => {
    await mountImageLab();

    const container = panels.querySelector('.oa-image-lab-generate') as HTMLElement;
    const dropzone = panels.querySelector('.oa-reference-dropzone') as HTMLElement;
    expect(container).not.toBeNull();

    const dragEnterEvent = createDragEvent('dragenter', [createFile('test.png', 'image/png')]);
    container.dispatchEvent(dragEnterEvent);
    await nextTick();

    expect(panels.querySelector('.oa-reference-zone.dragging')).not.toBeNull();
    expect(dropzone.classList.contains('dragging')).toBe(true);
    expect(dropzone.textContent).toContain(t('dropHint'));

    // Entering a child element increments depth without resetting dragging state
    const childDragEnter = createDragEvent('dragenter', [createFile('test.png', 'image/png')]);
    dropzone.dispatchEvent(childDragEnter);
    await nextTick();
    expect(dropzone.classList.contains('dragging')).toBe(true);

    // Leaving child element decrements depth but container is still active
    const childDragLeave = createDragEvent('dragleave', [createFile('test.png', 'image/png')]);
    dropzone.dispatchEvent(childDragLeave);
    await nextTick();
    expect(dropzone.classList.contains('dragging')).toBe(true);

    // Leaving container resets dragging state
    const containerDragLeave = createDragEvent('dragleave', [createFile('test.png', 'image/png')]);
    container.dispatchEvent(containerDragLeave);
    await nextTick();
    expect(panels.querySelector('.oa-reference-zone.dragging')).toBeNull();
  });

  it('supports one-shot drag and drop of multiple reference images at once', async () => {
    await mountImageLab();

    const container = panels.querySelector('.oa-image-lab-generate') as HTMLElement;
    const files = [
      createFile('photo1.png', 'image/png'),
      createFile('photo2.jpg', 'image/jpeg'),
      createFile('photo3.webp', 'image/webp'),
    ];

    const dropEvent = createDragEvent('drop', files);
    container.dispatchEvent(dropEvent);
    await settle();

    // All 3 images should be rendered as thumbnails
    const thumbnails = panels.querySelectorAll('.oa-reference');
    expect(thumbnails.length).toBe(3);

    // Since 3 < 5, compact add button is also displayed
    const addTile = panels.querySelector('.oa-reference-add-tile');
    expect(addTile).not.toBeNull();
    expect(addTile?.textContent).toContain(t('referenceImageAdd'));

    // Reference count in label shows 3/5
    expect(panels.querySelector('.oa-reference-count')?.textContent).toBe('3/5');
  });

  it('caps at maximum 5 reference images when dropping more than remaining slots', async () => {
    await mountImageLab();

    const container = panels.querySelector('.oa-image-lab-generate') as HTMLElement;
    const files = [
      createFile('img1.png', 'image/png'),
      createFile('img2.png', 'image/png'),
      createFile('img3.png', 'image/png'),
      createFile('img4.png', 'image/png'),
      createFile('img5.png', 'image/png'),
      createFile('img6.png', 'image/png'),
    ];

    container.dispatchEvent(createDragEvent('drop', files));
    await settle();

    // Only 5 are added
    const thumbnails = panels.querySelectorAll('.oa-reference');
    expect(thumbnails.length).toBe(5);

    // Limit warning flash is displayed
    const flash = panels.querySelector('.oa-drawer-flash');
    expect(flash?.textContent).toContain(t('referenceImageLimit', { count: 5 }));

    // Add button is hidden when max reached
    expect(panels.querySelector('.oa-reference-add-tile')).toBeNull();
  });

  it('rejects non-image files with an error message', async () => {
    await mountImageLab();

    const container = panels.querySelector('.oa-image-lab-generate') as HTMLElement;
    const files = [createFile('document.pdf', 'application/pdf')];

    container.dispatchEvent(createDragEvent('drop', files));
    await settle();

    expect(panels.querySelectorAll('.oa-reference').length).toBe(0);
    const flash = panels.querySelector('.oa-drawer-flash');
    expect(flash?.textContent).toContain(t('imageNotAnImage'));
  });

  it('supports clipboard pasting of image files', async () => {
    await mountImageLab();

    const container = panels.querySelector('.oa-image-lab-generate') as HTMLElement;
    const pastedFile = createFile('screenshot.png', 'image/png');

    container.dispatchEvent(createPasteEvent([pastedFile]));
    await settle();

    expect(panels.querySelectorAll('.oa-reference').length).toBe(1);
    expect(panels.querySelector('.oa-reference-count')?.textContent).toBe('1/5');
  });

  it('removes an attached reference image when clicking the remove button', async () => {
    await mountImageLab();

    const container = panels.querySelector('.oa-image-lab-generate') as HTMLElement;
    container.dispatchEvent(createDragEvent('drop', [
      createFile('a.png', 'image/png'),
      createFile('b.png', 'image/png'),
    ]));
    await settle();

    expect(panels.querySelectorAll('.oa-reference').length).toBe(2);

    const removeBtn = panels.querySelector('.oa-reference-remove') as HTMLButtonElement;
    removeBtn.click();
    await settle();

    expect(panels.querySelectorAll('.oa-reference').length).toBe(1);
    expect(panels.querySelector('.oa-reference-count')?.textContent).toBe('1/5');
  });
});
