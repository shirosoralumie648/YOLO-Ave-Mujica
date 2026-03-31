import { useQuery } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { fetchTask } from "./api";

export function TaskDetailPage() {
  const { taskId = "0" } = useParams();
  const query = useQuery({
    queryKey: ["task", taskId],
    queryFn: () => fetchTask(taskId),
  });

  if (query.isLoading) {
    return <section><h1>Task Detail</h1><p>Loading task...</p></section>;
  }
  if (query.isError || !query.data) {
    return <section><h1>Task Detail</h1><p>Failed to load task.</p></section>;
  }

  const task = query.data;
  return (
    <section>
      <h1>{task.title}</h1>
      <dl>
        <dt>Status</dt>
        <dd>{task.status}</dd>
        <dt>Priority</dt>
        <dd>{task.priority}</dd>
        <dt>Assignee</dt>
        <dd>{task.assignee || "unassigned"}</dd>
      </dl>
    </section>
  );
}
