/* Hallmark · component: confirm-delete · genre: dashboard · theme: Glass */
import { AlertDialog, Button, useOverlayState } from "@heroui/react";
import { Icon } from "@iconify/react";
import type { ReactNode } from "react";

/**
 * Danger-tinted trigger button paired with a controlled AlertDialog confirmation.
 * The dialog stays open (with controls disabled) while `isPending`, so a failed
 * delete surfaces its toast without the dialog vanishing; on success the caller
 * navigates away, which unmounts it.
 */
export function ConfirmDeleteButton({
  triggerLabel,
  heading,
  body,
  confirmLabel,
  isDisabled,
  isPending,
  onConfirm,
}: {
  triggerLabel: string;
  heading: string;
  body: ReactNode;
  confirmLabel: string;
  isDisabled?: boolean;
  isPending?: boolean;
  onConfirm: () => void;
}) {
  const state = useOverlayState();
  return (
    <>
      <Button size="sm" variant="outline" isDisabled={isDisabled} onPress={state.open}>
        <Icon icon="lucide:trash-2" className="mr-1.5 size-4 text-danger" />
        {triggerLabel}
      </Button>
      <AlertDialog.Backdrop isOpen={state.isOpen} onOpenChange={state.setOpen}>
        <AlertDialog.Container>
          <AlertDialog.Dialog className="sm:max-w-[420px]">
            <AlertDialog.CloseTrigger />
            <AlertDialog.Header>
              <AlertDialog.Icon status="danger" />
              <AlertDialog.Heading>{heading}</AlertDialog.Heading>
            </AlertDialog.Header>
            <AlertDialog.Body>{body}</AlertDialog.Body>
            <AlertDialog.Footer>
              <Button slot="close" variant="tertiary" isDisabled={isPending}>
                Cancel
              </Button>
              <Button variant="danger" isDisabled={isPending} onPress={onConfirm}>
                {confirmLabel}
              </Button>
            </AlertDialog.Footer>
          </AlertDialog.Dialog>
        </AlertDialog.Container>
      </AlertDialog.Backdrop>
    </>
  );
}
