/* Hallmark · component: stint-actions · genre: dashboard · theme: Glass */
import { useNavigate } from "@tanstack/react-router";
import { ConfirmDeleteButton } from "~/components/ConfirmDeleteButton";
import { useStintDelete } from "~/utils/mutations";

/**
 * Delete a single stint. Disabled while the stint is still recording. On
 * success, navigates back to the parent session.
 */
export function DeleteStintButton({
  id,
  sessionId,
  disabled,
}: {
  id: string;
  sessionId: string;
  disabled?: boolean;
}) {
  const { mutate, isPending } = useStintDelete();
  const navigate = useNavigate();
  return (
    <ConfirmDeleteButton
      triggerLabel="Delete"
      heading="Delete this stint?"
      body={
        <p>
          This permanently deletes the stint and its captured Parquet file. This can&rsquo;t be
          undone.
        </p>
      }
      confirmLabel="Delete stint"
      isDisabled={disabled}
      isPending={isPending}
      onConfirm={() =>
        mutate(
          { id, sessionId },
          { onSuccess: () => void navigate({ to: "/sessions/$sessionId", params: { sessionId } }) },
        )
      }
    />
  );
}
