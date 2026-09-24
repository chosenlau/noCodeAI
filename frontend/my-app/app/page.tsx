"use client"
import AddTodo from "@/components/AddTodo"
import TodoList from "@/components/TodoList"
import TodoItem from "@/components/TodoItem"
import { Todo } from "@/components/type"
import { useState } from "react"




export default function Home() {
  const [todos, setTodos] = useState<Todo[]>([])
  const [filteredTodos, setFilteredTodos] = useState<'all' | 'active' | 'completed'>('all')

  const addTodo = (text: string) => {
    const newTodo = {
      id: Date.now(),
      title: text,
      completed: false
    }
    setTodos([...todos, newTodo])
  }

  const deleteTodo = (id: number) => {
    const newTodos = todos.filter((todo) => todo.id !== id)
    setTodos(newTodos)
  }

  const getFilteredTodos = () => {
    switch (filteredTodos) {
      case 'active':
        return todos.filter((todo) => !todo.completed)
      case 'completed':
        return todos.filter((todo) => todo.completed)
      default:
        return todos
    }
  }

  const toggleTodo = (id: number) => {
    setTodos(
      todos.map((todo) => {
        if (todo.id === id) {
          todo.completed = !todo.completed
        }
        return todo
      })
    )
  }
  return (
    // 1. 全屏灰色背景，垂直方向 padding 8 个单位 (32px)
    <div >
      {/* 2. 主容器：最大宽度 28rem (448px)，水平居中 (mx-auto)，白色卡片背景，带圆角和阴影 */}
      <div className="max-w-md mx-auto bg-white p-6 rounded-xl shadow-md space-y-6">

        {/* 3. 标题：文字放大居中，加粗，主色调灰黑 */}
        <h1 className="text-2xl font-bold text-center text-gray-800 tracking-wide">
          TodoList
        </h1>

        <AddTodo addTodo={addTodo} />
        <TodoList todos={getFilteredTodos()} deleteTodo={deleteTodo} toggleTodo={toggleTodo} />
      </div>
    </div>
  );
}
