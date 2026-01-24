import { createSignal, Show } from "solid-js";

const [previewSrc, setPreviewSrc] = createSignal<string | null>(null);
const [previewAlt, setPreviewAlt] = createSignal<string>("");

export function openPreview(src: string, alt: string) {
  setPreviewSrc(src);
  setPreviewAlt(alt);
  document.body.style.overflow = "hidden";
}

export function closePreview() {
  setPreviewSrc(null);
  setPreviewAlt("");
  document.body.style.overflow = "";
}

export function PreviewImage(props: {
  src: string;
  alt: string;
  class?: string;
  loading?: "lazy" | "eager";
  width?: number;
  height?: number;
}) {
  return (
    <button
      type="button"
      class={`cursor-zoom-in border-0 bg-transparent p-0 ${props.class ?? ""}`}
      onClick={() => openPreview(props.src, props.alt)}
      aria-haspopup="dialog"
      aria-label={`View larger: ${props.alt}`}
    >
      <img
        src={props.src}
        alt={props.alt}
        class="block w-full"
        loading={props.loading}
        width={props.width}
        height={props.height}
      />
    </button>
  );
}

export function ImagePreviewModal() {
  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Escape") {
      closePreview();
    }
  };

  const src = () => previewSrc();

  return (
    <Show when={src()}>
      {(imgSrc) => (
        <div
          class="fixed inset-0 z-[100] flex items-center justify-center bg-nord0/95 p-4 backdrop-blur-sm"
          onClick={closePreview}
          onKeyDown={handleKeyDown}
          role="dialog"
          aria-modal="true"
          aria-label={previewAlt()}
        >
          {/* Close button */}
          <button
            type="button"
            class="absolute right-4 top-4 rounded-full bg-nord1 p-2 text-nord4 transition-colors hover:bg-nord2 hover:text-nord8"
            onClick={closePreview}
            aria-label="Close preview"
          >
            <svg
              class="h-6 w-6"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              aria-hidden="true"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                stroke-width="2"
                d="M6 18L18 6M6 6l12 12"
              />
            </svg>
          </button>

          {/* Image */}
          <img
            src={imgSrc()}
            alt={previewAlt()}
            class="max-h-[90vh] max-w-[90vw] rounded-lg shadow-2xl"
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => e.stopPropagation()}
          />

          {/* Alt text caption */}
          <Show when={previewAlt()}>
            <p class="absolute bottom-4 left-1/2 -translate-x-1/2 rounded-md bg-nord1/90 px-4 py-2 text-sm text-nord4">
              {previewAlt()}
            </p>
          </Show>
        </div>
      )}
    </Show>
  );
}
