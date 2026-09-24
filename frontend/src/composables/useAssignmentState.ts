import { ref } from "vue";
import { api } from "../api";
import type { Assignment } from "../types";

export function useAssignmentState(reload: () => Promise<void>) {
  const actionError = ref("");
  const busyAssignment = ref<string | null>(null);

  async function update(assignment: Assignment, personalDueAt: string | null, completionStatus: "todo" | "completed") {
    if (busyAssignment.value) return;
    busyAssignment.value = assignment.id;
    actionError.value = "";
    try {
      await api(`/assignments/${assignment.id}/my-state`, {
        method: "PUT",
        body: JSON.stringify({ personal_due_at: personalDueAt, completion_status: completionStatus }),
      });
      await reload();
    } catch (error) {
      actionError.value = error instanceof Error ? error.message : "保存个人状态失败";
    } finally {
      busyAssignment.value = null;
    }
  }

  const toggle = (assignment: Assignment) => update(
    assignment,
    assignment.personal_due_at,
    assignment.completion_status === "completed" ? "todo" : "completed",
  );
  const savePlan = (assignment: Assignment, value: string | null) => update(
    assignment,
    value,
    assignment.completion_status,
  );

  return { actionError, busyAssignment, toggle, savePlan };
}
