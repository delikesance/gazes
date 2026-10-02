import { ChevronDown } from "lucide-react";
import type { SelectHTMLAttributes } from "react";
export function Select({children, ...props}:SelectHTMLAttributes<HTMLSelectElement>) {
 return <span className="select-control"><select {...props}>{children}</select><ChevronDown size={16} aria-hidden="true" /></span>;
}
