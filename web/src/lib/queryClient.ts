import { QueryClient } from "@tanstack/react-query";

/** The app's one query cache: shared by React (main.tsx) and route loaders (router.tsx). */
export const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, refetchOnWindowFocus: true } },
});
