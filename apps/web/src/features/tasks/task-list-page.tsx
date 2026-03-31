import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { fetchTasks } from "./api";

export function TaskListPage() {
  const { projectId = "1" } = useParams();
  const query = useQuery({
    queryKey: ["tasks", projectId],
    queryFn: () => fetchTasks(projectId),
  });

  if (query.isLoading) {
    return <section><h1>Tasks</h1><p>Loading tasks...</p></section>;
  }
  if (query.isError || !query.data) {
    return <section><h1>Tasks</h1><p>Failed to load tasks.</p></section>;
  }

  return (
    <section>
      <h1>Task List</h1>
      <ul>
        {query.data.items.map((task) => (
          <li key={task.id}>
            <Link to={`/projects/${projectId}/tasks/${task.id}`}>{task.title}</Link>
            <span> {task.status}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
