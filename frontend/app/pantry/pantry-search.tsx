"use client";

import MobileHeader from "@/components/MobileHeader";
import {
  getImageUrl,
  getIngredients,
  IngredientItem,
  IngredientSearchResponse,
  IngredientSearchResult,
  searchRecipesByIngredients,
} from "@/lib/api";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

const STORAGE_KEY = "pantry.ingredients";
const MAX_INGREDIENTS = 30;
const CATEGORY_ORDER = [
  // 「調味料」はここに出るのは基本調味料以外（豆板醤など）。基本調味料は /ingredients で除外済み
  "肉", "魚介", "野菜", "きのこ", "豆・大豆製品", "卵・乳製品", "穀物・麺", "加工品", "調味料", "その他",
];

function loadSavedIngredients(): string[] {
  try {
    const saved = JSON.parse(localStorage.getItem(STORAGE_KEY) || "[]");
    return Array.isArray(saved) ? saved.filter((v) => typeof v === "string") : [];
  } catch {
    return [];
  }
}

// 食材から作れるレシピを探す画面の中身（認証は呼び出し側の page.tsx で行う）
export default function PantrySearch() {
  const [vocabulary, setVocabulary] = useState<IngredientItem[]>([]);
  const [selected, setSelected] = useState<string[]>(loadSavedIngredients);
  const [input, setInput] = useState("");
  const [result, setResult] = useState<IngredientSearchResponse | null>(null);
  const [searching, setSearching] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [imageUrls, setImageUrls] = useState<Record<string, string>>({});
  const requestIdRef = useRef(0);

  useEffect(() => {
    getIngredients()
      .then(setVocabulary)
      .catch((err) => console.error("Failed to load ingredients:", err));
  }, []);

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(selected));
    } catch {
      // 保存できなくても検索は続ける
    }
  }, [selected]);

  const loadImageUrls = useCallback(async (results: IngredientSearchResult[]) => {
    const keys = results
      .map((r) => r.recipe.imageKeys?.[0])
      .filter((key): key is string => !!key);
    const entries = await Promise.all(
      keys.map(async (key) => {
        try {
          return [key, await getImageUrl(key)] as const;
        } catch {
          return null;
        }
      })
    );
    setImageUrls((prev) => {
      const next = { ...prev };
      for (const entry of entries) {
        if (entry) next[entry[0]] = entry[1];
      }
      return next;
    });
  }, []);

  // 食材が変わるたびに自動で検索する（AIを使わないので一瞬で返る）
  useEffect(() => {
    const requestId = ++requestIdRef.current;
    if (selected.length === 0) {
      setResult(null);
      setError(null);
      setSearching(false);
      return;
    }
    const timer = setTimeout(async () => {
      try {
        setSearching(true);
        setError(null);
        const response = await searchRecipesByIngredients(selected);
        if (requestId !== requestIdRef.current) return;
        setResult(response);
        loadImageUrls(response.results);
      } catch (err) {
        if (requestId !== requestIdRef.current) return;
        console.error("Failed to search recipes:", err);
        setError("検索に失敗しました。時間をおいてもう一度お試しください。");
      } finally {
        if (requestId === requestIdRef.current) setSearching(false);
      }
    }, 300);
    return () => clearTimeout(timer);
  }, [selected, loadImageUrls]);

  const addIngredients = (text: string) => {
    const names = text
      .split(/[、,，\s]+/)
      .map((s) => s.trim())
      .filter(Boolean);
    if (names.length === 0) return;
    setSelected((prev) => {
      const next = [...prev];
      for (const name of names) {
        if (!next.includes(name) && next.length < MAX_INGREDIENTS) next.push(name);
      }
      return next;
    });
    setInput("");
  };

  const removeIngredient = (name: string) =>
    setSelected((prev) => prev.filter((n) => n !== name));

  const suggestions = useMemo(() => {
    const query = input.trim();
    if (!query) return [];
    return vocabulary
      .filter(
        (item) =>
          !selected.includes(item.name) &&
          (item.name.includes(query) || item.aliases.some((a) => a.includes(query)))
      )
      .slice(0, 8);
  }, [input, vocabulary, selected]);

  const frequentByCategory = useMemo(() => {
    const top = vocabulary
      .filter((item) => item.usageCount > 0 && !selected.includes(item.name))
      .slice(0, 20);
    return CATEGORY_ORDER.map((category) => ({
      category,
      items: top.filter((item) => item.category === category),
    })).filter((group) => group.items.length > 0);
  }, [vocabulary, selected]);

  const sections = result
    ? [
        { title: "今すぐ作れる", items: result.results.filter((r) => r.missing.length === 0) },
        { title: "あと1品で作れる", items: result.results.filter((r) => r.missing.length === 1) },
        { title: "あと2品で作れる", items: result.results.filter((r) => r.missing.length === 2) },
      ].filter((s) => s.items.length > 0)
    : [];

  return (
    <div>
      <MobileHeader title="食材から探す" />
      <main className="px-[18px] pt-[22px] pb-6 lg:max-w-2xl lg:mx-auto flex flex-col gap-[14px]">
        <div>
          <h2 className="text-[19px] font-extrabold text-[#2B2723] mb-1">
            家にある食材は？
          </h2>
          <p className="text-[13.5px] leading-[1.5] text-[#8A8178]">
            入れた食材で作れる保存済みレシピを探します。塩・しょうゆなどの基本調味料は入れなくて大丈夫です。
          </p>
        </div>

        {/* 入力 */}
        <div className="bg-white border border-[#EEE4D5] rounded-[18px] p-4 flex flex-col gap-3">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              addIngredients(input);
            }}
            className="flex gap-2"
          >
            <input
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="例: 鶏肉、玉ねぎ"
              enterKeyHint="done"
              className="flex-1 min-w-0 px-3 py-3 text-base border border-[#E7DBC7] rounded-[13px] focus:ring-2 focus:ring-amber-200 focus:border-amber-200"
            />
            <button
              type="submit"
              disabled={!input.trim() || selected.length >= MAX_INGREDIENTS}
              className="shrink-0 px-4 rounded-[13px] bg-[#E38A29] text-white font-bold disabled:bg-stone-300"
            >
              追加
            </button>
          </form>

          {suggestions.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {suggestions.map((item) => (
                <button
                  key={item.name}
                  type="button"
                  onClick={() => addIngredients(item.name)}
                  className="px-3 py-1.5 rounded-full border border-[#E7DBC7] text-sm text-[#5A5248] hover:bg-[#FBEFDD]"
                >
                  + {item.name}
                </button>
              ))}
            </div>
          )}

          {selected.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {selected.map((name) => (
                <span
                  key={name}
                  className="inline-flex items-center gap-1 pl-3 pr-1 py-1 rounded-full bg-[#FBEFDD] text-sm font-medium text-[#B26B18]"
                >
                  {name}
                  <button
                    type="button"
                    onClick={() => removeIngredient(name)}
                    aria-label={`${name}を外す`}
                    className="w-6 h-6 flex items-center justify-center rounded-full hover:bg-[#F5DEB9]"
                  >
                    ×
                  </button>
                </span>
              ))}
              <button
                type="button"
                onClick={() => setSelected([])}
                className="px-2 py-1 text-sm text-[#8A8178] underline"
              >
                すべて外す
              </button>
            </div>
          )}
        </div>

        {/* よく使う食材 */}
        {frequentByCategory.length > 0 && (
          <div className="flex flex-col gap-2">
            <h3 className="text-sm font-bold text-[#6F675C]">よく使う食材</h3>
            {frequentByCategory.map((group) => (
              <div key={group.category} className="flex flex-wrap items-center gap-2">
                <span className="text-xs text-[#A79C8B] w-full">{group.category}</span>
                {group.items.map((item) => (
                  <button
                    key={item.name}
                    type="button"
                    onClick={() => addIngredients(item.name)}
                    className="px-3 py-1.5 rounded-full bg-white border border-[#EEE4D5] text-sm text-[#5A5248] hover:border-[#E3D8C4]"
                  >
                    + {item.name}
                  </button>
                ))}
              </div>
            ))}
          </div>
        )}

        {/* 結果 */}
        {error && (
          <div className="p-3 rounded-lg text-sm bg-red-50 text-red-800">{error}</div>
        )}

        {result && result.unknownInputs.length > 0 && (
          <p className="text-xs text-[#8A8178]">
            {result.unknownInputs.map((n) => `「${n}」`).join("")}
            は登録されていない食材のため、検索に使っていません。候補から選ぶと確実です。
          </p>
        )}

        {searching && !result && (
          <p className="text-center text-sm text-stone-500 py-6">探しています…</p>
        )}

        {result && sections.length === 0 && (
          <p className="text-center text-sm text-stone-500 py-6">
            作れそうなレシピが見つかりませんでした
          </p>
        )}

        {sections.map((section) => (
          <section key={section.title} className="flex flex-col gap-2">
            <h3 className="text-[15px] font-extrabold text-[#2B2723]">
              {section.title}
              <span className="ml-1 text-sm font-medium text-[#8A8178]">
                {section.items.length}件
              </span>
            </h3>
            {section.items.map(({ recipe, missing }) => {
              const imageKey = recipe.imageKeys?.[0];
              return (
                <Link
                  key={recipe.recipeId}
                  href={`/recipes/${recipe.recipeId}`}
                  className="flex items-center gap-3 bg-white border border-[#EEE4D5] rounded-[14px] p-3 hover:border-[#E3D8C4]"
                >
                  <div className="w-16 h-16 shrink-0 rounded-[10px] bg-[#FBEFDD]/60 overflow-hidden">
                    {imageKey && imageUrls[imageKey] && (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img
                        src={imageUrls[imageKey]}
                        alt={recipe.title}
                        className="w-full h-full object-cover"
                      />
                    )}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="font-bold text-[#2B2723] line-clamp-2">{recipe.title}</p>
                    {missing.length > 0 ? (
                      <p className="text-xs text-[#C77B1F] mt-1">
                        足りない: {missing.join("、")}
                      </p>
                    ) : (
                      <p className="text-xs text-[#5C6B4A] mt-1">手持ちの食材で作れます</p>
                    )}
                  </div>
                </Link>
              );
            })}
          </section>
        ))}

        {result && result.unprocessedCount > 0 && (
          <p className="text-xs text-[#A79C8B]">
            食材が未登録のレシピが{result.unprocessedCount}件あり、検索の対象外になっています。
          </p>
        )}
      </main>
    </div>
  );
}
