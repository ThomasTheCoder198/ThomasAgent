import { redirect } from "next/navigation";

import { HOME_ROUTE } from "@/features/auth/paths";

export default function RootPage() {
  redirect(HOME_ROUTE);
}
