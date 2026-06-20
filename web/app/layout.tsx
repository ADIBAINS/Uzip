import "./styles.css";
import { AuthProvider } from "./auth";
import { Shell } from "./shell";

export const metadata = { title: "Uzip", description: "Multi-tenant URL analytics" };

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body>
        <AuthProvider><Shell>{children}</Shell></AuthProvider>
      </body>
    </html>
  );
}
