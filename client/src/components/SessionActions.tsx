/* Hallmark · component: session-actions · genre: dashboard · theme: Glass */
import { Button } from "@heroui/react";
import { Icon } from "@iconify/react";
import { useSessionDownsample, useSessionPin } from "~/utils/mutations";

/**
 * Icon-only pin toggle. Warning-coloured pin when pinned, muted when not.
 * Must render as a sibling of (never nested inside) the row's navigation
 * anchor — an interactive control inside an `<a>` is invalid.
 */
export function PinToggle({ id, pinned }: { id: string; pinned: boolean }) {
  const { mutate, isPending } = useSessionPin();
  return (
    <Button
      isIconOnly
      size="sm"
      variant="tertiary"
      isDisabled={isPending}
      aria-label={pinned ? "Unpin session" : "Pin session"}
      aria-pressed={pinned}
      onPress={() => mutate({ id, pinned: !pinned })}
    >
      <Icon
        icon="lucide:pin"
        className={pinned ? "size-4 text-warning-soft-foreground" : "size-4 text-muted"}
      />
    </Button>
  );
}

/**
 * Downsample action. The endpoint returns 501 today; the mutation's error
 * branch surfaces that as an info toast (see `useSessionDownsample`). Disabled
 * once a session is already downsampled.
 */
export function DownsampleButton({ id, downsampled }: { id: string; downsampled: boolean }) {
  const { mutate, isPending } = useSessionDownsample();
  return (
    <Button
      size="sm"
      variant="outline"
      isDisabled={isPending || downsampled}
      onPress={() => mutate(id)}
    >
      <Icon icon="lucide:minimize-2" className="mr-1.5 size-4" />
      {downsampled ? "Downsampled" : "Downsample"}
    </Button>
  );
}
