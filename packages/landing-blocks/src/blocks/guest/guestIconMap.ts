import type { Component } from 'svelte';
import {
  Briefcase,
  GraduationCap,
  Plane,
  TrendingUp,
  Users,
  Heart,
  Globe,
  MapPin,
  FileCheck,
  Calculator,
  Bot,
  Sparkles,
  Star,
  ClipboardCheck,
  Wrench,
  BookOpen,
  LayoutGrid,
  Home,
  Mail,
  Phone,
  Clock,
} from 'lucide-svelte';

const iconMap: Record<string, unknown> = {
  Briefcase,
  GraduationCap,
  Plane,
  TrendingUp,
  Users,
  Heart,
  Globe,
  MapPin,
  FileCheck,
  Calculator,
  Bot,
  Sparkles,
  Star,
  ClipboardCheck,
  Wrench,
  BookOpen,
  LayoutGrid,
  Home,
  Mail,
  Phone,
  Clock,
};

export function getLucideIcon(name: string | undefined): Component | null {
  if (!name) return null;
  return (iconMap[name] as unknown as Component) ?? null;
}
